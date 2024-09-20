package delete_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"url-shortener/internal/http-server/handlers/url/delete"
	"url-shortener/internal/http-server/handlers/url/delete/mocks"
	"url-shortener/internal/lib/api/response"
	"url-shortener/internal/lib/logger/handlers/slogdiscard"
	"url-shortener/internal/storage"
)

func TestDeleteHandlers(t *testing.T) {
	cases := []struct {
		name      string
		alias     string
		respError string
		mockError error
	}{
		{
			name:  "Success",
			alias: "test_alias",
		},
		{
			name:      "Alias not provided",
			alias:     "",
			respError: "alias is required",
		},
		{
			name:      "DeleteURL Error",
			alias:     "test_alias",
			respError: "failed to delete url",
			mockError: errors.New("unexpected error"),
		},
		{
			name:      "URL not found",
			alias:     "nonexistent_alias",
			respError: "url not found",
			mockError: storage.ErrURLNotFound,
		},
	}

	for _, tc := range cases {

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			urlDeleterMock := mocks.NewURLDeleter(t)

			if tc.respError == "" || tc.mockError != nil {
				urlDeleterMock.On("DeleteURL", tc.alias).Return(tc.mockError).Once()
			}

			handler := delete.New(slogdiscard.NewDiscardLogger(), urlDeleterMock)

			input := map[string]string{"alias": tc.alias}
			inputData, _ := json.Marshal(input)

			req, err := http.NewRequest(http.MethodDelete, "/delete", bytes.NewBuffer(inputData))
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)

			var resp response.Response
			_ = json.Unmarshal(rr.Body.Bytes(), &resp)

			require.Equal(t, tc.respError, resp.Error)
		})
	}
}
