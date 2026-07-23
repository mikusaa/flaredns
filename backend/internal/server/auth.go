package server

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/mikusaa/flaredns/backend/internal/security"
	"github.com/gin-gonic/gin"
)

func (a *App) login(c *gin.Context) {
	if !a.limiter.Allow("password:" + c.ClientIP()) {
		fail(c, http.StatusTooManyRequests, "rate_limited", "登录尝试过于频繁", nil)
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &input) {
		return
	}
	user, err := a.store.UserByUsername(c.Request.Context(), strings.TrimSpace(input.Username))
	if err != nil {
		if err == sql.ErrNoRows {
			time.Sleep(150 * time.Millisecond)
			fail(c, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误", nil)
			return
		}
		internal(c, err)
		return
	}
	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		fail(c, http.StatusTooManyRequests, "account_locked", "登录失败次数过多，请稍后再试", nil)
		return
	}
	if !security.VerifyPassword(user.PasswordHash, input.Password) {
		_ = a.store.RecordLoginFailure(c.Request.Context(), user.ID)
		fail(c, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误", nil)
		return
	}
	_ = a.store.ClearLoginFailures(c.Request.Context(), user.ID)
	raw, session, err := a.store.CreateSession(c.Request.Context(), user.ID, a.cfg.SessionTTL, true)
	if err != nil {
		internal(c, err)
		return
	}
	a.setCookie(c, raw, session.ExpiresAt)
	ok(c, gin.H{"username": user.Username, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (a *App) sessionInfo(c *gin.Context) {
	s := currentSession(c)
	passkeys, _ := a.store.ListPasskeys(c.Request.Context(), s.UserID)
	ok(c, gin.H{"username": s.Username, "csrf_token": s.CSRFToken, "expires_at": s.ExpiresAt, "passkey_count": len(passkeys), "public_url": a.cfg.PublicURL, "rp_id": a.cfg.RPID})
}

func (a *App) logout(c *gin.Context) {
	raw, _ := c.Get("session_token")
	_ = a.store.DeleteSession(c.Request.Context(), raw.(string))
	a.clearCookie(c)
	ok(c, gin.H{"logged_out": true})
}

func (a *App) reauthenticate(c *gin.Context) {
	var input struct {
		Password string `json:"password"`
	}
	if !bindJSON(c, &input) {
		return
	}
	s := currentSession(c)
	user, err := a.store.UserByID(c.Request.Context(), s.UserID)
	if err != nil {
		internal(c, err)
		return
	}
	if !security.VerifyPassword(user.PasswordHash, input.Password) {
		fail(c, http.StatusUnauthorized, "invalid_credentials", "密码错误", nil)
		return
	}
	if err := a.store.MarkReauthenticated(c.Request.Context(), s.ID); err != nil {
		internal(c, err)
		return
	}
	s.ReauthenticatedAt = time.Now()
	ok(c, gin.H{"reauthenticated_until": time.Now().Add(5 * time.Minute)})
}

func (a *App) changePassword(c *gin.Context) {
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !bindJSON(c, &input) {
		return
	}
	s := currentSession(c)
	user, err := a.store.UserByID(c.Request.Context(), s.UserID)
	if err != nil {
		internal(c, err)
		return
	}
	if !security.VerifyPassword(user.PasswordHash, input.CurrentPassword) {
		fail(c, http.StatusUnauthorized, "invalid_credentials", "当前密码错误", nil)
		return
	}
	hash, err := security.HashPassword(input.NewPassword)
	if err != nil {
		fail(c, http.StatusBadRequest, "weak_password", "新密码至少需要 12 个字符", map[string]string{"new_password": err.Error()})
		return
	}
	if err := a.store.ChangePassword(c.Request.Context(), s.UserID, s.ID, hash); err != nil {
		internal(c, err)
		return
	}
	_ = a.store.AddAudit(c.Request.Context(), s.UserID, s.Username, "修改密码", "user", "", s.Username, nil, nil, true, "", c.ClientIP())
	ok(c, gin.H{"changed": true})
}
