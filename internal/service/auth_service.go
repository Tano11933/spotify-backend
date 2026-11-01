package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	jwtpkg "spotify-backend/pkg/jwt"
)

const bcryptCost = 12

const minPasswordLength = 8

const resetTokenBytes = 32

var (
	ErrEmailAlreadyUsed   = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrWeakPassword       = fmt.Errorf("password must be at least %d characters", minPasswordLength)
	ErrPasswordTooLong    = errors.New("password must not exceed 72 bytes")
	ErrUserNotFound       = errors.New("user not found")
	ErrSessionNotFound    = errors.New("login session not found")
)

type PasswordResetMailer interface {
	SendPasswordReset(ctx context.Context, to, name, token string) error
}

type UserStore interface {
	Create(ctx context.Context, user *model.User) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
}

type TokenStore interface {
	SessionGeneration(context.Context, uuid.UUID) (string, error)
	CreateSession(context.Context, uuid.UUID, string, model.StoredLoginSession) (bool, error)
	GetSession(context.Context, uuid.UUID, uuid.UUID) (*model.StoredLoginSession, error)
	RotateSession(context.Context, uuid.UUID, uuid.UUID, string, string, string, time.Duration) (bool, error)
	ListSessions(context.Context, uuid.UUID, uuid.UUID) ([]model.LoginSession, error)
	RevokeSession(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	RevokeAllSessions(context.Context, uuid.UUID, uuid.UUID) error
	StoreResetToken(ctx context.Context, token string, userID uuid.UUID, ttl time.Duration) error
	ConsumeResetToken(ctx context.Context, token string) (uuid.UUID, error)
}

type AuthService struct {
	userRepo  UserStore
	tokenRepo TokenStore
	jwt       *jwtpkg.Manager
	mailer    PasswordResetMailer

	resetTokenTTL time.Duration

	dummyHash []byte
}

func NewAuthService(
	userRepo UserStore,
	tokenRepo TokenStore,
	jwtManager *jwtpkg.Manager,
	mailer PasswordResetMailer,
	resetTokenTTL time.Duration,
) *AuthService {
	dummy, err := bcrypt.GenerateFromPassword([]byte("timing-equalizer"), bcryptCost)
	if err != nil {
		log.Printf("warning: could not precompute dummy hash: %v", err)
	}

	return &AuthService{
		userRepo:      userRepo,
		tokenRepo:     tokenRepo,
		jwt:           jwtManager,
		mailer:        mailer,
		resetTokenTTL: resetTokenTTL,
		dummyHash:     dummy,
	}
}

func (s *AuthService) Register(ctx context.Context, req model.RegisterRequest) (*model.User, error) {
	email := normalizeEmail(req.Email)

	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}

	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("check existing email: %w", err)
	}
	if exists {
		return nil, ErrEmailAlreadyUsed
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return nil, ErrPasswordTooLong
		}
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &model.User{
		Name:         strings.TrimSpace(req.Name),
		Email:        email,
		PasswordHash: string(hash),

		Role: model.RoleUser,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, ErrEmailAlreadyUsed
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

func (s *AuthService) Login(ctx context.Context, req model.LoginRequest, devices ...model.LoginDevice) (*model.AuthResponse, error) {
	email := normalizeEmail(req.Email)

	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(req.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}

	generation, err := s.tokenRepo.SessionGeneration(ctx, user.ID)
	if err != nil {
		return nil, fmt.Errorf("get session generation: %w", err)
	}
	passwordHash := user.PasswordHash
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {

		return nil, ErrInvalidCredentials
	}

	latest, err := s.userRepo.FindByID(ctx, user.ID)
	if err != nil || latest.PasswordHash != passwordHash {
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("verify login account: %w", err)
		}
		return nil, ErrInvalidCredentials
	}
	device := model.LoginDevice{}
	if len(devices) > 0 {
		device = devices[0]
	}
	tokens, err := s.issueSession(ctx, latest, generation, device)
	if err != nil {
		return nil, err
	}

	return &model.AuthResponse{User: latest, Tokens: tokens}, nil
}

// Refresh menukar refresh token yang valid dengan sepasang token baru.
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*model.TokenPair, error) {
	claims, err := s.jwt.ParseRefreshToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	userID, err := claims.UserID()
	if err != nil {
		return nil, ErrInvalidToken
	}
	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil || sessionID == uuid.Nil {
		return nil, ErrInvalidToken
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}

	stored, err := s.tokenRepo.GetSession(ctx, userID, sessionID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, fmt.Errorf("get login session: %w", err)
	}
	credential := secretDigest(user.PasswordHash)
	if stored.CredentialHash != credential {
		return nil, ErrInvalidToken
	}
	tokens, err := s.generateTokens(user, sessionID)
	if err != nil {
		return nil, err
	}
	rotated, err := s.tokenRepo.RotateSession(ctx, userID, sessionID, secretDigest(refreshToken), secretDigest(tokens.RefreshToken), credential, s.jwt.RefreshTTL())
	if err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}
	if !rotated {
		return nil, ErrInvalidToken
	}
	return tokens, nil
}

func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID, sessionIDs ...uuid.UUID) error {
	if len(sessionIDs) == 0 || sessionIDs[0] == uuid.Nil {
		return nil
	}
	if err := s.tokenRepo.RevokeSession(ctx, userID, uuid.Nil, sessionIDs[0]); err != nil {
		return fmt.Errorf("revoke current session: %w", err)
	}
	return nil
}

func (s *AuthService) GetProfile(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return user, nil
}

func (s *AuthService) ForgotPassword(ctx context.Context, req model.ForgotPasswordRequest) error {
	email := normalizeEmail(req.Email)

	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("find user by email: %w", err)
	}

	token, err := generateResetToken()
	if err != nil {
		return fmt.Errorf("generate reset token: %w", err)
	}

	if err := s.tokenRepo.StoreResetToken(ctx, token, user.ID, s.resetTokenTTL); err != nil {
		return fmt.Errorf("store reset token: %w", err)
	}

	s.sendResetEmailAsync(user.Email, user.Name, token)
	return nil
}

func (s *AuthService) sendResetEmailAsync(email, name, token string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("recovered panic while sending reset email: %v", r)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := s.mailer.SendPasswordReset(ctx, email, name, token); err != nil {

			log.Printf("failed to send password reset email to %s: %v", email, err)
		}
	}()
}

// ResetPassword menyelesaikan alur reset dengan token dari email.
func (s *AuthService) ResetPassword(ctx context.Context, req model.ResetPasswordRequest) error {
	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}

	userID, err := s.tokenRepo.ConsumeResetToken(ctx, req.Token)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrInvalidToken
		}
		return fmt.Errorf("consume reset token: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		if errors.Is(err, bcrypt.ErrPasswordTooLong) {
			return ErrPasswordTooLong
		}
		return fmt.Errorf("hash password: %w", err)
	}

	if err := s.userRepo.UpdatePassword(ctx, userID, string(hash)); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// User dihapus setelah token dibuat.
			return ErrInvalidToken
		}
		return fmt.Errorf("update password: %w", err)
	}

	if err := s.tokenRepo.RevokeAllSessions(ctx, userID, uuid.Nil); err != nil {
		return fmt.Errorf("revoke sessions after password reset: %w", err)
	}

	return nil
}

func (s *AuthService) issueSession(ctx context.Context, user *model.User, generation string, device model.LoginDevice) (*model.TokenPair, error) {
	sessionID := uuid.New()
	tokens, err := s.generateTokens(user, sessionID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	agent := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, device.UserAgent))
	if len(agent) > 256 {
		agent = agent[:256]
	}
	ip := ""
	if parsed := net.ParseIP(device.IPAddress); parsed != nil {
		ip = parsed.String()
	}
	stored := model.StoredLoginSession{Session: model.LoginSession{ID: sessionID, UserAgent: string(agent), IPAddress: ip, CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(s.jwt.RefreshTTL())}, RefreshHash: secretDigest(tokens.RefreshToken), CredentialHash: secretDigest(user.PasswordHash)}
	created, err := s.tokenRepo.CreateSession(ctx, user.ID, generation, stored)
	if err != nil {
		return nil, fmt.Errorf("store login session: %w", err)
	}
	if !created {
		return nil, ErrInvalidCredentials
	}
	return tokens, nil
}

func (s *AuthService) generateTokens(user *model.User, sessionID uuid.UUID) (*model.TokenPair, error) {
	accessToken, err := s.jwt.GenerateAccessToken(user.ID, string(user.Role), sessionID)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.jwt.GenerateRefreshToken(user.ID, string(user.Role), sessionID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	return &model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.jwt.AccessTTL().Seconds()),
	}, nil
}

func secretDigest(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func (s *AuthService) validateSession(ctx context.Context, userID, currentID uuid.UUID) error {
	stored, err := s.tokenRepo.GetSession(ctx, userID, currentID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	if stored.CredentialHash != secretDigest(user.PasswordHash) {
		return ErrInvalidToken
	}
	return nil
}

func sessionError(err error) error {
	if errors.Is(err, repository.ErrSessionInvalid) {
		return ErrInvalidToken
	}
	if errors.Is(err, repository.ErrNotFound) {
		return ErrSessionNotFound
	}
	return err
}

func (s *AuthService) ListSessions(ctx context.Context, userID, currentID uuid.UUID) ([]model.LoginSession, error) {
	if err := s.validateSession(ctx, userID, currentID); err != nil {
		return nil, err
	}
	sessions, err := s.tokenRepo.ListSessions(ctx, userID, currentID)
	if err != nil {
		return nil, sessionError(err)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].LastUsedAt.Equal(sessions[j].LastUsedAt) {
			return sessions[i].ID.String() < sessions[j].ID.String()
		}
		return sessions[i].LastUsedAt.After(sessions[j].LastUsedAt)
	})
	return sessions, nil
}
func (s *AuthService) RevokeSession(ctx context.Context, userID, currentID, targetID uuid.UUID) error {
	if err := s.validateSession(ctx, userID, currentID); err != nil {
		return err
	}
	return sessionError(s.tokenRepo.RevokeSession(ctx, userID, currentID, targetID))
}
func (s *AuthService) RevokeAllSessions(ctx context.Context, userID, currentID uuid.UUID) error {
	if err := s.validateSession(ctx, userID, currentID); err != nil {
		return err
	}
	return sessionError(s.tokenRepo.RevokeAllSessions(ctx, userID, currentID))
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < minPasswordLength {
		return ErrWeakPassword
	}
	if len(password) > 72 {
		return ErrPasswordTooLong
	}
	return nil
}

// generateResetToken membuat token acak untuk link reset password.
func generateResetToken() (string, error) {
	b := make([]byte, resetTokenBytes)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
