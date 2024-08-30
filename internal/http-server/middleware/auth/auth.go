// auth/auth.go

package auth

import (
	"errors"
	"github.com/dgrijalva/jwt-go"
	"golang.org/x/exp/slog"
	"net/http"
	"strings"
	"url-shortener/internal/lib/logger/sl"
	"url-shortener/internal/storage"
	"url-shortener/internal/storage/postgres"
)

const (
	AuthHeader = "Authorization"
	Bearer     = "Bearer"
)

var (
	ErrInvalidToken = errors.New("invalid token")
)

type Authenticator struct {
	SecretKey string
	Logger    *slog.Logger
	Storage   *postgres.Storage
}

func NewAuthenticator(secretKey string, logger *slog.Logger, storage *postgres.Storage) *Authenticator {
	return &Authenticator{
		SecretKey: secretKey,
		Logger:    logger,
		Storage:   storage,
	}
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get(AuthHeader)
		if strings.HasPrefix(authHeader, Bearer) {
			tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, Bearer+" "))

			// Проверяем валидность токена с использованием Storage
			isValid, err := a.Storage.IsTokenValid(tokenString)
			if err != nil {
				if errors.Is(err, storage.ErrTokenNotFound) {
					a.Logger.Error("token not found", sl.Err(err))
					http.Error(w, "Token not found: The token you provided was not found", http.StatusUnauthorized)
					return
				}
				if errors.Is(err, storage.ErrTokenExpired) {
					a.Logger.Error("token is expired", sl.Err(err))
					http.Error(w, "Token expired: The token you provided has expired", http.StatusForbidden)
					return
				}
				a.Logger.Error("error validation token", sl.Err(err))
				http.Error(w, "Internal Server Error: An error occurred while validating the token", http.StatusInternalServerError)
				return
			}

			if !isValid {
				a.Logger.Error("token validation failed", sl.Err(ErrInvalidToken))
				http.Error(w, "Token is invalid: The token you provided is not valid", http.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				// Убедитесь, что метод подписи соответствует ожидаемому
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					a.Logger.Error("unexpected signing method", sl.Err(ErrInvalidToken))
					return nil, ErrInvalidToken
				}
				return []byte(a.SecretKey), nil
			})
			if err != nil {
				// Логируем ошибку разбора токена
				a.Logger.Error("token parsing failed", sl.Err(err))
				http.Error(w, "Unauthorized: The token could not be parsed", http.StatusUnauthorized)
				return
			}

			// Если токен валиден, выполняем следующий обработчик
			if !token.Valid {
				a.Logger.Error("token not valid", sl.Err(ErrInvalidToken))
				http.Error(w, "Token not valid: The token you provided is not valid", http.StatusUnauthorized)
				return
			}
		} else {
			a.Logger.Info("no token provided")
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
