package queue

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// RecoverExpired moves up to 100 abandoned reservations to retry/dead-letter.
// Reserve calls it even when ready jobs exist, so recovery needs no extra process.
func (q *Queue) RecoverExpired(ctx context.Context) (int, error) {
	return recoverScript.Run(ctx, q.rdb, q.keys(), MaxAttempts,
		BaseRetryDelay.Milliseconds(), MaxRetryDelay.Milliseconds()).Int()
}

var recoverScript = redis.NewScript(redisNow + `
local expired = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now, 'LIMIT', 0, 100)
for _, reservation in ipairs(expired) do
  local raw = redis.call('HGET', KEYS[2], reservation)
  if raw then
    local job = cjson.decode(raw)
    job.last_error = 'processing lease expired'
    raw = cjson.encode(job)
    if job.attempt >= tonumber(ARGV[1]) then
      redis.call('RPUSH', KEYS[5], raw)
    else
      local delay = math.min(tonumber(ARGV[2]) * 2 ^ (job.attempt - 1), tonumber(ARGV[3]))
      redis.call('ZADD', KEYS[4], now + delay, raw)
    end
    redis.call('HDEL', KEYS[2], reservation)
  end
  redis.call('ZREM', KEYS[3], reservation)
end
return #expired
`)
