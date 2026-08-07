package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
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
	StoreRefreshToken(ctx context.Context, userID uuid.UUID, token string, ttl time.Duration) error
	GetRefreshToken(ctx context.Context, userID uuid.UUID) (string, error)
	DeleteRefreshToken(ctx context.Context, userID uuid.UUID) error
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

func (s *AuthService) Login(ctx context.Context, req model.LoginRequest) (*model.AuthResponse, error) {
	email := normalizeEmail(req.Email)

	user, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(req.Password))
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("find user by email: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {

		return nil, ErrInvalidCredentials
	}

	tokens, err := s.issueTokens(ctx, user)
	if err != nil {
		return nil, err
	}

	return &model.AuthResponse{User: user, Tokens: tokens}, nil
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

	stored, err := s.tokenRepo.GetRefreshToken(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("get stored refresh token: %w", err)
	}

	if stored != refreshToken {
		return nil, ErrInvalidToken
	}

	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}

	return s.issueTokens(ctx, user)
}

func (s *AuthService) Logout(ctx context.Context, userID uuid.UUID) error {
	if err := s.tokenRepo.DeleteRefreshToken(ctx, userID); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
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

	if err := s.tokenRepo.DeleteRefreshToken(ctx, userID); err != nil {

		log.Printf("failed to revoke refresh token for user %s after reset: %v", userID, err)
	}

	return nil
}

func (s *AuthService) issueTokens(ctx context.Context, user *model.User) (*model.TokenPair, error) {
	accessToken, err := s.jwt.GenerateAccessToken(user.ID, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.jwt.GenerateRefreshToken(user.ID, string(user.Role))
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	if err := s.tokenRepo.StoreRefreshToken(ctx, user.ID, refreshToken, s.jwt.RefreshTTL()); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.jwt.AccessTTL().Seconds()),
	}, nil
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
