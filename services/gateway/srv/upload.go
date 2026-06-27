package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/google/uuid"
	"github.com/tencentyun/cos-go-sdk-v5"
)

// UploadHandler provides COS pre-signed upload URLs.
type UploadHandler struct {
	cos       *cos.Client
	secretID  string
	secretKey string
}

func NewUploadHandler(cfg config.COSConfig) *UploadHandler {
	if cfg.BucketURL == "" {
		return &UploadHandler{} // COS not configured
	}
	bucketURL, err := url.Parse(cfg.BucketURL)
	if err != nil {
		return &UploadHandler{}
	}
	return &UploadHandler{
		cos: cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
			Transport: &cos.AuthorizationTransport{
				SecretID:  cfg.SecretID,
				SecretKey: cfg.SecretKey,
			},
		}),
		secretID:  cfg.SecretID,
		secretKey: cfg.SecretKey,
	}
}

func (h *UploadHandler) RegisterMux(ctx context.Context, mx *http.ServeMux) {
	mx.HandleFunc("GET /upload/token", h.handleGetToken)
}

func (h *UploadHandler) handleGetToken(w http.ResponseWriter, r *http.Request) {
	if h.cos == nil {
		http.Error(w, "COS not configured", http.StatusNotImplemented)
		return
	}

	ext := r.URL.Query().Get("ext")
	if ext == "" {
		ext = ".jpg"
	}
	objectKey := fmt.Sprintf("uploads/%s/%s%s", time.Now().Format("2006/01/02"), uuid.New().String(), ext)

	presigned, err := h.cos.Object.GetPresignedURL(
		r.Context(), http.MethodPut, objectKey,
		h.secretID, h.secretKey, time.Hour, nil,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"object_key": objectKey,
		"upload_url": presigned.String(),
	})
}
