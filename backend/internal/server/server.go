package server

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mikusaa/flaredns/backend/internal/cloudflare"
	"github.com/mikusaa/flaredns/backend/internal/config"
	"github.com/mikusaa/flaredns/backend/internal/security"
	"github.com/mikusaa/flaredns/backend/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

//go:embed web/*
var webFS embed.FS

const sessionCookie = "flaredns_session"

type App struct {
	cfg        config.Config
	store      *store.Store
	cipher     *security.Cipher
	cloudflare *cloudflare.Client
	webauthn   *webauthn.WebAuthn
	limiter    *loginLimiter
}

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func New(cfg config.Config, s *store.Store, cipher *security.Cipher) (*App, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: "FlareDNS",
		RPOrigins:     []string{cfg.PublicURL},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute},
		},
	})
	if err != nil {
		return nil, err
	}
	return &App{cfg: cfg, store: s, cipher: cipher, cloudflare: cloudflare.New(cfg.CloudflareAPIURL), webauthn: wa, limiter: newLoginLimiter()}, nil
}

func (a *App) Router() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), a.requestLogger(), a.securityHeaders(), a.originGuard())
	if len(a.cfg.TrustedProxies) == 0 {
		_ = r.SetTrustedProxies(nil)
	} else {
		_ = r.SetTrustedProxies(a.cfg.TrustedProxies)
	}
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/readyz", a.ready)

	api := r.Group("/api")
	api.POST("/auth/login", a.login)
	api.POST("/auth/passkey/login/options", a.passkeyLoginOptions)
	api.POST("/auth/passkey/login/finish", a.passkeyLoginFinish)

	authed := api.Group("")
	authed.Use(a.requireSession(), a.csrfGuard())
	authed.GET("/auth/session", a.sessionInfo)
	authed.POST("/auth/logout", a.logout)
	authed.PUT("/auth/password", a.changePassword)
	authed.POST("/auth/reauth", a.reauthenticate)
	authed.GET("/passkeys", a.listPasskeys)
	authed.POST("/passkeys/register/options", a.passkeyRegisterOptions)
	authed.POST("/passkeys/register/finish", a.passkeyRegisterFinish)
	authed.PATCH("/passkeys/:id", a.renamePasskey)
	authed.DELETE("/passkeys/:id", a.deletePasskey)

	authed.GET("/tokens", a.listTokens)
	authed.POST("/tokens", a.createToken)
	authed.DELETE("/tokens/:id", a.deleteToken)
	authed.POST("/tokens/:id/verify", a.verifyToken)
	authed.POST("/tokens/:id/sync", a.syncToken)
	authed.GET("/zones", a.listZones)
	authed.PUT("/zones/:id/default", a.setDefaultZone)
	authed.GET("/zones/:id/records", a.listRecords)
	authed.POST("/zones/:id/records", a.createRecord)
	authed.PUT("/zones/:id/records/:recordID", a.updateRecord)
	authed.DELETE("/zones/:id/records/:recordID", a.deleteRecord)
	authed.POST("/zones/:id/records/batch", a.batchRecords)
	authed.GET("/logs", a.listLogs)

	a.serveFrontend(r)
	return r
}

func (a *App) StartCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = a.store.CleanupExpiredAuth(context.Background())
			}
		}
	}()
}

func (a *App) ready(c *gin.Context) {
	if err := a.store.DB.PingContext(c.Request.Context()); err != nil {
		fail(c, http.StatusServiceUnavailable, "database_unavailable", "数据库不可用", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func ok(c *gin.Context, data any)      { c.JSON(http.StatusOK, gin.H{"data": data}) }
func created(c *gin.Context, data any) { c.JSON(http.StatusCreated, gin.H{"data": data}) }
func fail(c *gin.Context, status int, code, message string, fields map[string]string) {
	c.AbortWithStatusJSON(status, gin.H{"error": apiError{Code: code, Message: message, Fields: fields}})
}
func internal(c *gin.Context, err error) {
	slog.Error("request failed", "path", c.Request.URL.Path, "error", err)
	fail(c, http.StatusInternalServerError, "internal_error", "服务器处理请求失败", nil)
}
func bindJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(c, http.StatusBadRequest, "invalid_request", "请求内容格式不正确", nil)
		return false
	}
	return true
}
func parseID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		fail(c, http.StatusBadRequest, "invalid_id", "资源 ID 不正确", nil)
		return 0, false
	}
	return id, true
}

func (a *App) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http request", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration", time.Since(start))
	}
}
func (a *App) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		c.Next()
	}
}
func (a *App) originGuard() gin.HandlerFunc {
	public, _ := url.Parse(a.cfg.PublicURL)
	expected := public.Scheme + "://" + public.Host
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			if origin := c.GetHeader("Origin"); origin != "" && !strings.EqualFold(strings.TrimRight(origin, "/"), expected) {
				fail(c, http.StatusForbidden, "invalid_origin", "请求来源不受信任", nil)
				return
			}
		}
		c.Next()
	}
}

func (a *App) requireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(sessionCookie)
		if err != nil {
			fail(c, http.StatusUnauthorized, "unauthenticated", "请先登录", nil)
			return
		}
		session, err := a.store.SessionByToken(c.Request.Context(), raw)
		if err != nil {
			a.clearCookie(c)
			fail(c, http.StatusUnauthorized, "session_expired", "登录已过期", nil)
			return
		}
		c.Set("session", session)
		c.Set("session_token", raw)
		c.Next()
	}
}
func (a *App) csrfGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
			s := currentSession(c)
			if c.GetHeader("X-CSRF-Token") == "" || c.GetHeader("X-CSRF-Token") != s.CSRFToken {
				fail(c, http.StatusForbidden, "invalid_csrf", "安全令牌已失效，请刷新页面", nil)
				return
			}
		}
		c.Next()
	}
}
func currentSession(c *gin.Context) *store.Session {
	value, _ := c.Get("session")
	return value.(*store.Session)
}
func (a *App) setCookie(c *gin.Context, raw string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: raw, Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, Expires: expires, MaxAge: int(time.Until(expires).Seconds())})
}
func (a *App) clearCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func (a *App) requireRecent(c *gin.Context) bool {
	if currentSession(c).ReauthenticatedAt.IsZero() || time.Since(currentSession(c).ReauthenticatedAt) > 5*time.Minute {
		fail(c, http.StatusForbidden, "reauthentication_required", "请先验证身份", nil)
		return false
	}
	return true
}

func (a *App) serveFrontend(r *gin.Engine) {
	root, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	assets, _ := fs.Sub(root, "assets")
	r.StaticFS("/assets", http.FS(assets))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			fail(c, http.StatusNotFound, "not_found", "接口不存在", nil)
			return
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			c.String(http.StatusServiceUnavailable, "FlareDNS frontend has not been built")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
}

type attempt struct {
	count int
	reset time.Time
}
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]attempt
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: map[string]attempt{}} }
func (l *loginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.attempts) >= 10_000 {
		for existingKey, existing := range l.attempts {
			if now.After(existing.reset) {
				delete(l.attempts, existingKey)
			}
		}
		if len(l.attempts) >= 10_000 {
			return false
		}
	}
	item := l.attempts[key]
	if now.After(item.reset) {
		item = attempt{reset: now.Add(time.Minute)}
	}
	item.count++
	l.attempts[key] = item
	return item.count <= 10
}
