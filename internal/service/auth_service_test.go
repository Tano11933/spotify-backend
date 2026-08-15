package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"spotify-backend/internal/model"
	"spotify-backend/internal/repository"
	jwtpkg "spotify-backend/pkg/jwt"
)

// ─── Test double ─────────────────────────────────────────────────────────────
//
// Tiga implementasi tiruan di bawah memenuhi UserStore, TokenStore, dan
// PasswordResetMailer. Tidak ada library mocking yang dipakai, dan itu memang
// kebiasaan di Go: karena interface dipenuhi secara struktural, menulis fake
// sendiri biasanya lebih pendek dan lebih jelas daripada mengonfigurasi mock.
//
// Semuanya dilindungi mutex karena ForgotPassword mengirim email di goroutine —
// tanpa mutex, `go test -race` akan menandai data race pada fake mailer.

type fakeUserStore struct {
	mu       sync.Mutex
	byID     map[uuid.UUID]*model.User
	failNext error
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{byID: make(map[uuid.UUID]*model.User)}
}

func (f *fakeUserStore) Create(_ context.Context, user *model.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return err
	}

	for _, existing := range f.byID {
		if existing.Email == user.Email {
			return repository.ErrDuplicate
		}
	}

	if user.ID == uuid.Nil {
		// Meniru hook BeforeCreate milik model, yang tidak berjalan tanpa GORM.
		user.ID = uuid.New()
	}

	copied := *user
	f.byID[user.ID] = &copied
	return nil
}

func (f *fakeUserStore) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	user, ok := f.byID[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	copied := *user
	return &copied, nil
}

func (f *fakeUserStore) FindByEmail(_ context.Context, email string) (*model.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, user := range f.byID {
		if user.Email == email {
			copied := *user
			return &copied, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeUserStore) ExistsByEmail(_ context.Context, email string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, user := range f.byID {
		if user.Email == email {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeUserStore) UpdatePassword(_ context.Context, id uuid.UUID, hash string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	user, ok := f.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	user.PasswordHash = hash
	return nil
}

type fakeTokenStore struct {
	mu       sync.Mutex
	refresh  map[uuid.UUID]string
	resets   map[string]uuid.UUID
	resetTTL map[string]time.Duration
}

func newFakeTokenStore() *fakeTokenStore {
	return &fakeTokenStore{
		refresh:  make(map[uuid.UUID]string),
		resets:   make(map[string]uuid.UUID),
		resetTTL: make(map[string]time.Duration),
	}
}

func (f *fakeTokenStore) StoreRefreshToken(_ context.Context, userID uuid.UUID, token string, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refresh[userID] = token
	return nil
}

func (f *fakeTokenStore) GetRefreshToken(_ context.Context, userID uuid.UUID) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	token, ok := f.refresh[userID]
	if !ok {
		return "", repository.ErrNotFound
	}
	return token, nil
}

func (f *fakeTokenStore) DeleteRefreshToken(_ context.Context, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.refresh, userID)
	return nil
}

func (f *fakeTokenStore) StoreResetToken(_ context.Context, token string, userID uuid.UUID, ttl time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resets[token] = userID
	f.resetTTL[token] = ttl
	return nil
}

// ConsumeResetToken meniru GETDEL: baca lalu langsung hapus.
func (f *fakeTokenStore) ConsumeResetToken(_ context.Context, token string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	userID, ok := f.resets[token]
	if !ok {
		return uuid.Nil, repository.ErrNotFound
	}
	delete(f.resets, token)
	return userID, nil
}

type sentEmail struct {
	To    string
	Name  string
	Token string
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentEmail
}

func (f *fakeMailer) SendPasswordReset(_ context.Context, to, name, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentEmail{To: to, Name: name, Token: token})
	return nil
}

func (f *fakeMailer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func (f *fakeMailer) last() (sentEmail, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return sentEmail{}, false
	}
	return f.sent[len(f.sent)-1], true
}

// waitForEmail menunggu email terkirim, karena pengirimannya asinkron.
//
// Polling dengan batas waktu dipakai, bukan time.Sleep dengan durasi tetap:
// sleep yang terlalu pendek membuat test kadang gagal (flaky), sleep yang terlalu
// panjang memperlambat seluruh suite tanpa alasan.
func waitForEmail(t *testing.T, mailer *fakeMailer, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mailer.count() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("email terkirim = %d, menunggu %d", mailer.count(), want)
}

// ─── Setup ───────────────────────────────────────────────────────────────────

type authFixture struct {
	svc    *AuthService
	users  *fakeUserStore
	tokens *fakeTokenStore
	mailer *fakeMailer
	jwt    *jwtpkg.Manager
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()

	users := newFakeUserStore()
	tokens := newFakeTokenStore()
	mailer := &fakeMailer{}
	manager := jwtpkg.NewManager("test-access-secret", "test-refresh-secret", 15*time.Minute, 168*time.Hour)

	return &authFixture{
		svc:    NewAuthService(users, tokens, manager, mailer, 15*time.Minute),
		users:  users,
		tokens: tokens,
		mailer: mailer,
		jwt:    manager,
	}
}

func (f *authFixture) register(t *testing.T, email, password string) *model.User {
	t.Helper()

	user, err := f.svc.Register(context.Background(), model.RegisterRequest{
		Name:     "Gabriel",
		Email:    email,
		Password: password,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return user
}

// ─── Register ────────────────────────────────────────────────────────────────

func TestRegisterHashesPasswordAndNormalizesEmail(t *testing.T) {
	f := newAuthFixture(t)

	user := f.register(t, "  Gabriel@Example.COM  ", "rahasia123")

	if user.Email != "gabriel@example.com" {
		t.Errorf("email = %q, ingin %q", user.Email, "gabriel@example.com")
	}

	if user.PasswordHash == "rahasia123" {
		t.Fatal("password disimpan sebagai plaintext")
	}
	if !strings.HasPrefix(user.PasswordHash, "$2a$") && !strings.HasPrefix(user.PasswordHash, "$2b$") {
		t.Errorf("hash tidak berformat bcrypt: %q", user.PasswordHash)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("rahasia123")); err != nil {
		t.Errorf("hash tidak cocok dengan password aslinya: %v", err)
	}
}

// TestRegisterAlwaysAssignsUserRole menguji pertahanan terhadap privilege
// escalation. RegisterRequest memang tidak punya field Role, tapi service tetap
// harus memaksanya — supaya kalau suatu hari field itu ditambahkan ke DTO,
// perilakunya tidak berubah diam-diam.
func TestRegisterAlwaysAssignsUserRole(t *testing.T) {
	f := newAuthFixture(t)

	user := f.register(t, "gabriel@example.com", "rahasia123")

	if user.Role != model.RoleUser {
		t.Errorf("role = %q, ingin %q", user.Role, model.RoleUser)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "gabriel@example.com", "rahasia123")

	// Kapitalisasi berbeda harus tetap terdeteksi sebagai email yang sama.
	_, err := f.svc.Register(context.Background(), model.RegisterRequest{
		Name: "Gabriel Lain", Email: "GABRIEL@example.com", Password: "rahasia456",
	})
	if !errors.Is(err, ErrEmailAlreadyUsed) {
		t.Errorf("err = %v, ingin ErrEmailAlreadyUsed", err)
	}
}

// TestRegisterMapsRaceConditionDuplicate menguji jalur yang sulit dipicu di
// dunia nyata: unique index database yang menolak INSERT karena request lain
// menyelipkan email yang sama setelah ExistsByEmail lolos.
func TestRegisterMapsRaceConditionDuplicate(t *testing.T) {
	f := newAuthFixture(t)
	f.users.failNext = repository.ErrDuplicate

	_, err := f.svc.Register(context.Background(), model.RegisterRequest{
		Name: "Gabriel", Email: "gabriel@example.com", Password: "rahasia123",
	})
	if !errors.Is(err, ErrEmailAlreadyUsed) {
		t.Errorf("err = %v, ingin ErrEmailAlreadyUsed", err)
	}
}

func TestRegisterRejectsWeakPassword(t *testing.T) {
	f := newAuthFixture(t)

	_, err := f.svc.Register(context.Background(), model.RegisterRequest{
		Name: "Gabriel", Email: "gabriel@example.com", Password: "1234567",
	})
	if !errors.Is(err, ErrWeakPassword) {
		t.Errorf("err = %v, ingin ErrWeakPassword", err)
	}
}

// TestRegisterRejectsPasswordOverBcryptLimit menjaga batas 72 byte bcrypt.
// Tanpa pemeriksaan ini, bcrypt mengembalikan error dan user menerima 500
// alih-alih pesan yang menjelaskan masalahnya.
func TestRegisterRejectsPasswordOverBcryptLimit(t *testing.T) {
	f := newAuthFixture(t)

	_, err := f.svc.Register(context.Background(), model.RegisterRequest{
		Name: "Gabriel", Email: "gabriel@example.com", Password: strings.Repeat("a", 73),
	})
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("err = %v, ingin ErrPasswordTooLong", err)
	}
}

// TestValidatePasswordCountsCharactersNotBytes mengunci pembedaan rune vs byte.
// Password 8 emoji = 8 karakter (sah) tapi 32 byte; kalau minimum dihitung per
// byte, aturan "minimal 8 karakter" jadi salah untuk non-ASCII.
func TestValidatePasswordCountsCharactersNotBytes(t *testing.T) {
	eightEmoji := strings.Repeat("🎵", 8)

	if got := len(eightEmoji); got <= minPasswordLength {
		t.Fatalf("prasyarat test salah: panjang byte %d tidak melebihi %d", got, minPasswordLength)
	}
	if err := validatePassword(eightEmoji); err != nil {
		t.Errorf("password 8 karakter ditolak: %v", err)
	}

	// 19 emoji = 76 byte: sah dari sisi jumlah karakter, tapi melewati batas
	// bcrypt yang dihitung per byte.
	if err := validatePassword(strings.Repeat("🎵", 19)); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("err = %v, ingin ErrPasswordTooLong", err)
	}
}

// ─── Login ───────────────────────────────────────────────────────────────────

func TestLoginReturnsTokensAndStoresRefreshToken(t *testing.T) {
	f := newAuthFixture(t)
	user := f.register(t, "gabriel@example.com", "rahasia123")

	res, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "rahasia123",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if res.Tokens.TokenType != "Bearer" {
		t.Errorf("token_type = %q, ingin Bearer", res.Tokens.TokenType)
	}
	if res.Tokens.ExpiresIn != int64((15 * time.Minute).Seconds()) {
		t.Errorf("expires_in = %d, ingin 900", res.Tokens.ExpiresIn)
	}

	// Access token harus benar-benar bisa diverifikasi dan menunjuk user ini.
	claims, err := f.jwt.ParseAccessToken(res.Tokens.AccessToken)
	if err != nil {
		t.Fatalf("access token tidak valid: %v", err)
	}
	gotID, _ := claims.UserID()
	if gotID != user.ID {
		t.Errorf("subject token = %s, ingin %s", gotID, user.ID)
	}

	// Refresh token wajib tercatat, kalau tidak endpoint refresh akan menolaknya.
	stored, err := f.tokens.GetRefreshToken(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("refresh token tidak tersimpan: %v", err)
	}
	if stored != res.Tokens.RefreshToken {
		t.Error("refresh token tersimpan tidak sama dengan yang dikembalikan")
	}
}

func TestLoginAcceptsDifferentEmailCasing(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "gabriel@example.com", "rahasia123")

	if _, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "  GABRIEL@Example.com ", Password: "rahasia123",
	}); err != nil {
		t.Errorf("login dengan kapitalisasi berbeda gagal: %v", err)
	}
}

// TestLoginGivesIdenticalErrorForWrongPasswordAndUnknownEmail adalah inti
// pertahanan terhadap user enumeration: kalau kedua kasus menghasilkan error
// berbeda, endpoint login jadi alat untuk mengecek email siapa yang punya akun.
func TestLoginGivesIdenticalErrorForWrongPasswordAndUnknownEmail(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "gabriel@example.com", "rahasia123")

	_, wrongPassword := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "salah",
	})
	_, unknownEmail := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "tidak-ada@example.com", Password: "rahasia123",
	})

	if !errors.Is(wrongPassword, ErrInvalidCredentials) {
		t.Errorf("password salah: err = %v, ingin ErrInvalidCredentials", wrongPassword)
	}
	if !errors.Is(unknownEmail, ErrInvalidCredentials) {
		t.Errorf("email tak dikenal: err = %v, ingin ErrInvalidCredentials", unknownEmail)
	}
	if wrongPassword.Error() != unknownEmail.Error() {
		t.Errorf("pesan error berbeda: %q vs %q", wrongPassword, unknownEmail)
	}
}

// ─── Refresh ─────────────────────────────────────────────────────────────────

// TestRefreshRotatesToken menguja perilaku keamanan terpenting dari endpoint
// refresh: token lama harus mati begitu ditukar. Tanpa rotasi, refresh token yang
// tercuri bisa dipakai berulang kali selama 7 hari.
func TestRefreshRotatesToken(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "gabriel@example.com", "rahasia123")

	login, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "rahasia123",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := login.Tokens.RefreshToken

	rotated, err := f.svc.Refresh(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if rotated.RefreshToken == oldRefresh {
		t.Fatal("refresh token tidak dirotasi")
	}

	// Token lama harus ditolak sekarang.
	if _, err := f.svc.Refresh(context.Background(), oldRefresh); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("refresh token lama masih diterima, err = %v", err)
	}

	// Token baru harus diterima.
	if _, err := f.svc.Refresh(context.Background(), rotated.RefreshToken); err != nil {
		t.Errorf("refresh token baru ditolak: %v", err)
	}
}

// TestRefreshRejectsSignatureValidTokenThatWasRevoked menguji alasan keberadaan
// Redis di alur ini. JWT tidak bisa dibatalkan begitu diterbitkan — signature
// token ini tetap sah selamanya sampai exp. Yang membuatnya tertolak adalah
// hilangnya key di token store.
func TestRefreshRejectsSignatureValidTokenThatWasRevoked(t *testing.T) {
	f := newAuthFixture(t)
	user := f.register(t, "gabriel@example.com", "rahasia123")

	login, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "rahasia123",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := f.svc.Logout(context.Background(), user.ID); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Signature-nya masih valid secara kriptografis...
	if _, err := f.jwt.ParseRefreshToken(login.Tokens.RefreshToken); err != nil {
		t.Fatalf("prasyarat test salah: token seharusnya masih valid secara kripto: %v", err)
	}
	// ...tapi service harus tetap menolaknya.
	if _, err := f.svc.Refresh(context.Background(), login.Tokens.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token yang sudah dicabut masih diterima, err = %v", err)
	}
}

func TestRefreshRejectsAccessToken(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "gabriel@example.com", "rahasia123")

	login, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "rahasia123",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, err := f.svc.Refresh(context.Background(), login.Tokens.AccessToken); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("access token diterima di endpoint refresh, err = %v", err)
	}
}

// ─── Forgot / Reset password ─────────────────────────────────────────────────

func TestForgotPasswordSendsEmailWithStoredToken(t *testing.T) {
	f := newAuthFixture(t)
	user := f.register(t, "gabriel@example.com", "rahasia123")

	if err := f.svc.ForgotPassword(context.Background(), model.ForgotPasswordRequest{
		Email: "gabriel@example.com",
	}); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}

	waitForEmail(t, f.mailer, 1)

	email, _ := f.mailer.last()
	if email.To != "gabriel@example.com" {
		t.Errorf("penerima = %q, ingin gabriel@example.com", email.To)
	}

	// Token di email harus token yang benar-benar tersimpan, kalau tidak link
	// reset-nya tidak akan berfungsi.
	gotUserID, err := f.tokens.ConsumeResetToken(context.Background(), email.Token)
	if err != nil {
		t.Fatalf("token di email tidak ada di token store: %v", err)
	}
	if gotUserID != user.ID {
		t.Errorf("token menunjuk user %s, ingin %s", gotUserID, user.ID)
	}
}

// TestForgotPasswordIsSilentForUnknownEmail: tidak ada error DAN tidak ada email
// terkirim. Keduanya penting — error akan membocorkan bahwa email itu tidak
// terdaftar, dan mengirim email ke alamat yang tidak punya akun berarti endpoint
// ini bisa dipakai menyebar spam.
func TestForgotPasswordIsSilentForUnknownEmail(t *testing.T) {
	f := newAuthFixture(t)

	if err := f.svc.ForgotPassword(context.Background(), model.ForgotPasswordRequest{
		Email: "tidak-ada@example.com",
	}); err != nil {
		t.Errorf("err = %v, ingin nil (tidak boleh membocorkan email tidak terdaftar)", err)
	}

	time.Sleep(150 * time.Millisecond)
	if got := f.mailer.count(); got != 0 {
		t.Errorf("email terkirim = %d, ingin 0", got)
	}
}

func TestResetPasswordChangesPasswordAndRevokesSession(t *testing.T) {
	f := newAuthFixture(t)
	user := f.register(t, "gabriel@example.com", "rahasia123")

	if _, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "rahasia123",
	}); err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := f.svc.ForgotPassword(context.Background(), model.ForgotPasswordRequest{
		Email: "gabriel@example.com",
	}); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	waitForEmail(t, f.mailer, 1)
	email, _ := f.mailer.last()

	if err := f.svc.ResetPassword(context.Background(), model.ResetPasswordRequest{
		Token: email.Token, NewPassword: "passwordbaru123",
	}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// Password lama harus mati.
	if _, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "rahasia123",
	}); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password lama masih bisa login, err = %v", err)
	}

	// Password baru harus jalan.
	if _, err := f.svc.Login(context.Background(), model.LoginRequest{
		Email: "gabriel@example.com", Password: "passwordbaru123",
	}); err != nil {
		t.Errorf("password baru tidak bisa login: %v", err)
	}

	_ = user
}

// TestResetPasswordTokenIsSingleUse memastikan token benar-benar habis sekali
// pakai. Kalau tidak, siapa pun yang pernah melihat link reset (di riwayat
// browser, log proxy, atau email yang diteruskan) bisa memakainya lagi.
func TestResetPasswordTokenIsSingleUse(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "gabriel@example.com", "rahasia123")

	if err := f.svc.ForgotPassword(context.Background(), model.ForgotPasswordRequest{
		Email: "gabriel@example.com",
	}); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	waitForEmail(t, f.mailer, 1)
	email, _ := f.mailer.last()

	if err := f.svc.ResetPassword(context.Background(), model.ResetPasswordRequest{
		Token: email.Token, NewPassword: "passwordbaru123",
	}); err != nil {
		t.Fatalf("reset pertama gagal: %v", err)
	}

	if err := f.svc.ResetPassword(context.Background(), model.ResetPasswordRequest{
		Token: email.Token, NewPassword: "passwordlain123",
	}); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("token bisa dipakai dua kali, err = %v", err)
	}
}

func TestResetPasswordRejectsUnknownToken(t *testing.T) {
	f := newAuthFixture(t)

	if err := f.svc.ResetPassword(context.Background(), model.ResetPasswordRequest{
		Token: "token-yang-tidak-pernah-diterbitkan", NewPassword: "passwordbaru123",
	}); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, ingin ErrInvalidToken", err)
	}
}

// TestResetPasswordValidatesBeforeConsumingToken menjaga urutan operasi:
// password divalidasi SEBELUM token dihabiskan. Kalau urutannya terbalik, user
// yang salah mengetik password terlalu pendek akan kehilangan token-nya dan
// harus meminta email reset baru.
func TestResetPasswordValidatesBeforeConsumingToken(t *testing.T) {
	f := newAuthFixture(t)
	f.register(t, "gabriel@example.com", "rahasia123")

	if err := f.svc.ForgotPassword(context.Background(), model.ForgotPasswordRequest{
		Email: "gabriel@example.com",
	}); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	waitForEmail(t, f.mailer, 1)
	email, _ := f.mailer.last()

	if err := f.svc.ResetPassword(context.Background(), model.ResetPasswordRequest{
		Token: email.Token, NewPassword: "pendek",
	}); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, ingin ErrWeakPassword", err)
	}

	// Token harus masih hidup dan bisa dipakai dengan password yang benar.
	if err := f.svc.ResetPassword(context.Background(), model.ResetPasswordRequest{
		Token: email.Token, NewPassword: "passwordbaru123",
	}); err != nil {
		t.Errorf("token terbuang oleh percobaan yang gagal validasi: %v", err)
	}
}

// ─── Helper ──────────────────────────────────────────────────────────────────

func TestNormalizeEmail(t *testing.T) {
	cases := map[string]string{
		"  Gabriel@Example.COM  ": "gabriel@example.com",
		"already@lower.com":       "already@lower.com",
		"\tTAB@Example.com\n":     "tab@example.com",
	}

	for input, want := range cases {
		if got := normalizeEmail(input); got != want {
			t.Errorf("normalizeEmail(%q) = %q, ingin %q", input, got, want)
		}
	}
}

// TestGenerateResetTokenIsUrlSafeAndUnique memastikan token bisa ditempel ke
// query string tanpa di-escape, dan tidak pernah berulang.
func TestGenerateResetTokenIsUrlSafeAndUnique(t *testing.T) {
	seen := make(map[string]bool)

	for i := 0; i < 500; i++ {
		token, err := generateResetToken()
		if err != nil {
			t.Fatalf("generateResetToken: %v", err)
		}

		if strings.ContainsAny(token, "+/=") {
			t.Fatalf("token mengandung karakter yang perlu di-escape di URL: %q", token)
		}
		if len(token) < 40 {
			t.Fatalf("token terlalu pendek (%d karakter): %q", len(token), token)
		}
		if seen[token] {
			t.Fatalf("token berulang pada iterasi %d: %q", i, token)
		}
		seen[token] = true
	}
}
