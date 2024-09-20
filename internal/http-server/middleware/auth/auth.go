// auth/auth.go

package auth

import (
	"encoding/json"
	"errors"
	"github.com/dgrijalva/jwt-go"
	"golang.org/x/exp/slog"
	"io"
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
	SSOURL    string
}

func NewAuthenticator(secretKey string, logger *slog.Logger, storage *postgres.Storage, ssoURL string) *Authenticator {
	return &Authenticator{
		SecretKey: secretKey,
		Logger:    logger,
		Storage:   storage,
		SSOURL:    ssoURL,
	}
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get(AuthHeader)
		if strings.HasPrefix(authHeader, Bearer) {
			tokenString := strings.TrimSpace(strings.TrimPrefix(authHeader, Bearer+" "))

			// Проверяем валидность токена с использованием Storage
			_, err := a.Storage.IsTokenValid(tokenString)
			if err != nil {
				if errors.Is(err, storage.ErrTokenExpired) {
					a.Logger.Warn("token is expired, attempting to refresh", sl.Err(err))

					// Попытка обновить токен через SSO
					newToken, refreshErr := a.refreshToken(tokenString)
					if refreshErr != nil {
						a.Logger.Error("failed to refresh token", sl.Err(refreshErr))
						http.Error(w, "Token expired and could not be refreshed", http.StatusForbidden)
						return
					}

					// Обновляем заголовок с новым токеном и продолжаем выполнение запроса
					r.Header.Set(AuthHeader, Bearer+" "+newToken)
					tokenString = newToken // Обновляем локальную переменную для дальнейшей проверки
				} else {
					a.Logger.Error("error validating token", sl.Err(err))
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
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
				a.Logger.Error("token parsing failed", sl.Err(err))
				http.Error(w, "Unauthorized: The token could not be parsed", http.StatusUnauthorized)
				return
			}

			// Если токен валиден, выполняем следующий обработчик
			if !token.Valid {
				a.Logger.Error("token not valid", sl.Err(ErrInvalidToken))
				http.Error(w, "Unauthorized: The token is not valid", http.StatusUnauthorized)
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

func (a *Authenticator) refreshToken(oldToken string) (string, error) {
	request, err := http.NewRequest("POST", a.SSOURL+"/refresh", nil)
	if err != nil {
		return "", err
	}

	request.Header.Set(AuthHeader, Bearer+" "+oldToken)

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", errors.New("failed to refresh token")
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	var jsonResponse map[string]string
	if err = json.Unmarshal(body, &jsonResponse); err != nil {
		return "", err
	}

	newToken, exists := jsonResponse["token"]
	if !exists {
		return "", errors.New("token not found in response")
	}

	return newToken, nil
}
