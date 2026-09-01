package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName = "fpan_session"
	authenticatedKey  = "authenticated"
)

func Authentication(sessions *Sessions) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authenticated := false

		sessionID, err := ctx.Cookie(sessionCookieName)
		if err == nil {
			authenticated = sessions.Valid(sessionID)
		}

		ctx.Set(authenticatedKey, authenticated)
		ctx.Next()
	}
}

func IsAuthenticated(ctx *gin.Context) bool {
	value, exists := ctx.Get(authenticatedKey)
	if !exists {
		return false
	}
	authenticated, ok := value.(bool)

	return ok && authenticated
}

func RequireAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if !IsAuthenticated(ctx) {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized * 10,
				"message": "authentication required",
			})
			return
		}

		ctx.Next()
	}
}

func SetSessionCookie(ctx *gin.Context, sessionID string, secure bool) {
	ctx.SetSameSite(http.SameSiteLaxMode)

	ctx.SetCookie(
		sessionCookieName,
		sessionID,
		int(sessionLifetime.Seconds()),
		"/",
		"",
		secure,
		true,
	)
}

func ClearSessionCookie(ctx *gin.Context, secure bool) {
	ctx.SetSameSite(http.SameSiteLaxMode)

	ctx.SetCookie(
		sessionCookieName,
		"",
		-1,
		"/",
		"",
		secure,
		true,
	)
}

func Logout(ctx *gin.Context, sessions *Sessions, secure bool) {
	sessionID, err := ctx.Cookie(sessionCookieName)
	if err == nil {
		sessions.Delete(sessionID)
	}

	ClearSessionCookie(ctx, secure)
}
