package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"spotify-backend/internal/model"
)

var ErrSessionInvalid = errors.New("login session is no longer active")

func sessionKey(userID uuid.UUID) string    { return fmt.Sprintf("auth:{%s}:sessions", userID) }
func generationKey(userID uuid.UUID) string { return fmt.Sprintf("auth:{%s}:generation", userID) }

type sessionRow struct {
	ID             uuid.UUID `json:"id"`
	UserAgent      string    `json:"user_agent"`
	IPAddress      string    `json:"ip_address"`
	CreatedAt      int64     `json:"created_at_ms"`
	LastUsedAt     int64     `json:"last_used_at_ms"`
	ExpiresAt      int64     `json:"expires_at_ms"`
	RefreshHash    string    `json:"refresh_hash"`
	CredentialHash string    `json:"credential_hash"`
}

func (row sessionRow) model() *model.StoredLoginSession {
	return &model.StoredLoginSession{Session: model.LoginSession{ID: row.ID, UserAgent: row.UserAgent, IPAddress: row.IPAddress, CreatedAt: time.UnixMilli(row.CreatedAt).UTC(), LastUsedAt: time.UnixMilli(row.LastUsedAt).UTC(), ExpiresAt: time.UnixMilli(row.ExpiresAt).UTC()}, RefreshHash: row.RefreshHash, CredentialHash: row.CredentialHash}
}

// Each script prunes expired sessions and preserves the longest remaining TTL.
const sessionLua = `
local now = tonumber(ARGV[1])
local function active(id)
 local raw = redis.call('HGET', KEYS[1], id)
 if not raw then return nil end
 local row = cjson.decode(raw)
 if row.expires_at_ms <= now then
  redis.call('HDEL', KEYS[1], id)
  return nil
 end
 return row
end
local function retain()
 local latest = 0
 local fields = redis.call('HKEYS', KEYS[1])
 for _, id in ipairs(fields) do
  local row = active(id)
  if row and row.expires_at_ms > latest then latest = row.expires_at_ms end
 end
 if latest > now then redis.call('PEXPIREAT', KEYS[1], latest)
 else redis.call('DEL', KEYS[1]) end
end
`

var createSessionScript = redis.NewScript(sessionLua + `
if (redis.call('GET', KEYS[2]) or '') ~= ARGV[2] then return 0 end
redis.call('HSET', KEYS[1], ARGV[3], ARGV[4])
retain()
return 1
`)
var getSessionScript = redis.NewScript(sessionLua + `
local row = active(ARGV[2])
retain()
if not row then return false end
return cjson.encode(row)
`)
var rotateSessionScript = redis.NewScript(sessionLua + `
local row = active(ARGV[2])
if not row or row.refresh_hash ~= ARGV[3] or row.credential_hash ~= ARGV[5] then retain(); return 0 end
row.refresh_hash = ARGV[4]
row.last_used_at_ms = now
row.expires_at_ms = now + tonumber(ARGV[6])
redis.call('HSET', KEYS[1], ARGV[2], cjson.encode(row))
retain()
return 1
`)
var listSessionsScript = redis.NewScript(sessionLua + `
if not active(ARGV[2]) then retain(); return {-1} end
retain()
local result = {1}
local rows = redis.call('HVALS', KEYS[1])
for _, raw in ipairs(rows) do table.insert(result, raw) end
return result
`)
var revokeSessionScript = redis.NewScript(sessionLua + `
if ARGV[2] ~= '' and not active(ARGV[2]) then retain(); return -1 end
local row = active(ARGV[3])
redis.call('HDEL', KEYS[1], ARGV[3])
retain()
if not row then return 0 end
return 1
`)
var revokeAllSessionsScript = redis.NewScript(sessionLua + `
if ARGV[2] ~= '' and not active(ARGV[2]) then retain(); return 0 end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], ARGV[3])
return 1
`)

func (r *TokenRepository) SessionGeneration(ctx context.Context, userID uuid.UUID) (string, error) {
	value, err := r.rdb.Get(ctx, generationKey(userID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return value, err
}
func (r *TokenRepository) CreateSession(ctx context.Context, userID uuid.UUID, generation string, stored model.StoredLoginSession) (bool, error) {
	s := stored.Session
	raw, err := json.Marshal(sessionRow{ID: s.ID, UserAgent: s.UserAgent, IPAddress: s.IPAddress, CreatedAt: s.CreatedAt.UnixMilli(), LastUsedAt: s.LastUsedAt.UnixMilli(), ExpiresAt: s.ExpiresAt.UnixMilli(), RefreshHash: stored.RefreshHash, CredentialHash: stored.CredentialHash})
	if err != nil {
		return false, err
	}
	result, err := createSessionScript.Run(ctx, r.rdb, []string{sessionKey(userID), generationKey(userID)}, time.Now().UnixMilli(), generation, s.ID.String(), string(raw)).Int()
	return result == 1, err
}
func (r *TokenRepository) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*model.StoredLoginSession, error) {
	raw, err := getSessionScript.Run(ctx, r.rdb, []string{sessionKey(userID)}, time.Now().UnixMilli(), sessionID.String()).Text()
	if errors.Is(err, redis.Nil) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var row sessionRow
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		return nil, err
	}
	return row.model(), nil
}
func (r *TokenRepository) RotateSession(ctx context.Context, userID, sessionID uuid.UUID, previous, next, credential string, ttl time.Duration) (bool, error) {
	if ttl.Milliseconds() <= 0 {
		return false, fmt.Errorf("session TTL must be positive")
	}
	result, err := rotateSessionScript.Run(ctx, r.rdb, []string{sessionKey(userID)}, time.Now().UnixMilli(), sessionID.String(), previous, next, credential, ttl.Milliseconds()).Int()
	return result == 1, err
}
func (r *TokenRepository) ListSessions(ctx context.Context, userID, currentID uuid.UUID) ([]model.LoginSession, error) {
	result, err := listSessionsScript.Run(ctx, r.rdb, []string{sessionKey(userID)}, time.Now().UnixMilli(), currentID.String()).Slice()
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || result[0].(int64) != 1 {
		return nil, ErrSessionInvalid
	}
	sessions := make([]model.LoginSession, 0, len(result)-1)
	for _, raw := range result[1:] {
		var row sessionRow
		if err := json.Unmarshal([]byte(raw.(string)), &row); err != nil {
			return nil, err
		}
		s := row.model().Session
		s.Current = s.ID == currentID
		sessions = append(sessions, s)
	}
	return sessions, nil
}
func (r *TokenRepository) RevokeSession(ctx context.Context, userID, currentID, targetID uuid.UUID) error {
	caller := ""
	if currentID != uuid.Nil {
		caller = currentID.String()
	}
	result, err := revokeSessionScript.Run(ctx, r.rdb, []string{sessionKey(userID)}, time.Now().UnixMilli(), caller, targetID.String()).Int()
	if err != nil {
		return err
	}
	if result < 0 {
		return ErrSessionInvalid
	}
	if result == 0 && currentID != uuid.Nil {
		return ErrNotFound
	}
	return nil
}
func (r *TokenRepository) RevokeAllSessions(ctx context.Context, userID, currentID uuid.UUID) error {
	caller := ""
	if currentID != uuid.Nil {
		caller = currentID.String()
	}
	result, err := revokeAllSessionsScript.Run(ctx, r.rdb, []string{sessionKey(userID), generationKey(userID)}, time.Now().UnixMilli(), caller, uuid.NewString()).Int()
	if err != nil {
		return err
	}
	if result != 1 {
		return ErrSessionInvalid
	}
	return nil
}
