package server

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mikusaa/flaredns/backend/internal/security"
	"github.com/mikusaa/flaredns/backend/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const challengeTTL = 3 * time.Minute

func (a *App) passkeyLoginOptions(c *gin.Context) {
	if !a.limiter.Allow("passkey:" + c.ClientIP()) {
		fail(c, http.StatusTooManyRequests, "rate_limited", "请求过于频繁", nil)
		return
	}
	user, err := a.store.UserByUsername(c.Request.Context(), "admin")
	if err != nil {
		internal(c, err)
		return
	}
	if len(user.Credentials) == 0 {
		fail(c, http.StatusBadRequest, "no_passkeys", "尚未注册 Passkey", nil)
		return
	}
	allowed := make([]protocol.CredentialDescriptor, len(user.Credentials))
	for i, credential := range user.Credentials {
		allowed[i] = credential.Descriptor()
	}
	options, sessionData, err := a.webauthn.BeginDiscoverableLogin(
		webauthn.WithUserVerification(protocol.VerificationRequired),
		webauthn.WithAllowedCredentials(allowed),
	)
	if err != nil {
		internal(c, err)
		return
	}
	id, _ := security.RandomToken(24)
	raw, _ := json.Marshal(sessionData)
	if err := a.store.SaveChallenge(c.Request.Context(), id, "login", nil, raw, time.Now().Add(challengeTTL)); err != nil {
		internal(c, err)
		return
	}
	ok(c, gin.H{"challenge_id": id, "options": options})
}

func (a *App) passkeyLoginFinish(c *gin.Context) {
	id := c.GetHeader("X-WebAuthn-Challenge-ID")
	if id == "" {
		fail(c, http.StatusBadRequest, "missing_challenge", "缺少 Passkey Challenge", nil)
		return
	}
	rawSession, _, err := a.store.ConsumeChallenge(c.Request.Context(), id, "login")
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid_challenge", "Passkey Challenge 已过期或被使用", nil)
		return
	}
	var sessionData webauthn.SessionData
	if err := json.Unmarshal(rawSession, &sessionData); err != nil {
		internal(c, err)
		return
	}
	var matched *store.User
	user, credential, err := a.webauthn.FinishPasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		found, lookupErr := a.store.UserByCredentialID(c.Request.Context(), rawID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		matched = found
		return found, nil
	}, sessionData, c.Request)
	if err != nil || matched == nil {
		slog.Warn("passkey login failed", "error", err, "user_matched", matched != nil)
		fail(c, http.StatusUnauthorized, "passkey_failed", "Passkey 验证失败", nil)
		return
	}
	_ = user
	if err := a.store.UpdatePasskeyUsage(c.Request.Context(), matched.ID, credential); err != nil {
		internal(c, err)
		return
	}
	if c.GetHeader("X-Reauth") == "1" {
		if existingRaw, cookieErr := c.Cookie(sessionCookie); cookieErr == nil {
			if existing, sessionErr := a.store.SessionByToken(c.Request.Context(), existingRaw); sessionErr == nil && existing.UserID == matched.ID {
				if err := a.store.MarkReauthenticated(c.Request.Context(), existing.ID); err != nil {
					internal(c, err)
					return
				}
				ok(c, gin.H{"reauthenticated_until": time.Now().Add(5 * time.Minute)})
				return
			}
		}
	}
	raw, session, err := a.store.CreateSession(c.Request.Context(), matched.ID, a.cfg.SessionTTL, true)
	if err != nil {
		internal(c, err)
		return
	}
	a.setCookie(c, raw, session.ExpiresAt)
	ok(c, gin.H{"username": matched.Username, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (a *App) listPasskeys(c *gin.Context) {
	items, err := a.store.ListPasskeys(c.Request.Context(), currentSession(c).UserID)
	if err != nil {
		internal(c, err)
		return
	}
	ok(c, items)
}

func (a *App) passkeyRegisterOptions(c *gin.Context) {
	if !a.requireRecent(c) {
		return
	}
	s := currentSession(c)
	user, err := a.store.UserByID(c.Request.Context(), s.UserID)
	if err != nil {
		internal(c, err)
		return
	}
	options, sessionData, err := a.webauthn.BeginRegistration(user,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
	if err != nil {
		internal(c, err)
		return
	}
	id, _ := security.RandomToken(24)
	raw, _ := json.Marshal(sessionData)
	if err := a.store.SaveChallenge(c.Request.Context(), id, "register", &s.UserID, raw, time.Now().Add(challengeTTL)); err != nil {
		internal(c, err)
		return
	}
	ok(c, gin.H{"challenge_id": id, "options": options})
}

func (a *App) passkeyRegisterFinish(c *gin.Context) {
	if !a.requireRecent(c) {
		return
	}
	id := c.GetHeader("X-WebAuthn-Challenge-ID")
	name := strings.TrimSpace(c.GetHeader("X-Passkey-Name"))
	if len(name) < 1 || len(name) > 60 {
		fail(c, http.StatusBadRequest, "invalid_name", "Passkey 名称应为 1 至 60 个字符", nil)
		return
	}
	rawSession, userID, err := a.store.ConsumeChallenge(c.Request.Context(), id, "register")
	if err != nil || userID == nil || *userID != currentSession(c).UserID {
		fail(c, http.StatusBadRequest, "invalid_challenge", "Passkey Challenge 已过期或被使用", nil)
		return
	}
	var sessionData webauthn.SessionData
	if err := json.Unmarshal(rawSession, &sessionData); err != nil {
		internal(c, err)
		return
	}
	user, err := a.store.UserByID(c.Request.Context(), *userID)
	if err != nil {
		internal(c, err)
		return
	}
	credential, err := a.webauthn.FinishRegistration(user, sessionData, c.Request)
	if err != nil {
		fail(c, http.StatusBadRequest, "passkey_failed", "Passkey 注册验证失败", nil)
		return
	}
	passkeyID, err := a.store.AddPasskey(c.Request.Context(), user.ID, name, credential)
	if err != nil {
		internal(c, err)
		return
	}
	_ = a.store.AddAudit(c.Request.Context(), user.ID, user.Username, "添加 Passkey", "passkey", strconv.FormatInt(passkeyID, 10), name, nil, gin.H{"name": name}, true, "", c.ClientIP())
	created(c, gin.H{"id": passkeyID, "name": name})
}

func (a *App) renamePasskey(c *gin.Context) {
	id, okID := parseID(c, "id")
	if !okID {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if !bindJSON(c, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len(input.Name) < 1 || len(input.Name) > 60 {
		fail(c, http.StatusBadRequest, "invalid_name", "名称应为 1 至 60 个字符", nil)
		return
	}
	s := currentSession(c)
	if err := a.store.RenamePasskey(c.Request.Context(), s.UserID, id, input.Name); err != nil {
		if err == sql.ErrNoRows {
			fail(c, http.StatusNotFound, "not_found", "Passkey 不存在", nil)
			return
		}
		internal(c, err)
		return
	}
	ok(c, gin.H{"renamed": true})
}

func (a *App) deletePasskey(c *gin.Context) {
	if !a.requireRecent(c) {
		return
	}
	id, okID := parseID(c, "id")
	if !okID {
		return
	}
	s := currentSession(c)
	if err := a.store.DeletePasskey(c.Request.Context(), s.UserID, id); err != nil {
		if err == sql.ErrNoRows {
			fail(c, http.StatusNotFound, "not_found", "Passkey 不存在", nil)
			return
		}
		internal(c, err)
		return
	}
	_ = a.store.AddAudit(c.Request.Context(), s.UserID, s.Username, "删除 Passkey", "passkey", c.Param("id"), "", nil, nil, true, "", c.ClientIP())
	ok(c, gin.H{"deleted": true})
}
