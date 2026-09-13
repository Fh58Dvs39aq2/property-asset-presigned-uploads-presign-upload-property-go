package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type storageSigner interface {
	PresignPut(context.Context, string, string, string, int64, string) (PresignedUpload, error)
}

type AssetUploadService struct {
	storage storageSigner
	bucket  string
	now     func() time.Time
}

func NewAssetUploadService(storage storageSigner, bucket string, now func() time.Time) *AssetUploadService {
	return &AssetUploadService{storage: storage, bucket: bucket, now: now}
}

type uploadIntent struct {
	AssetKind   string `json:"asset_kind"`
	PropertyID  string `json:"property_id"`
	RecordID    string `json:"record_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

type uploadPolicy struct {
	prefix       string
	maxBytes     int64
	contentTypes map[string]bool
}

var policies = map[string]uploadPolicy{
	"maintenance_request": {
		prefix:       "maintenance",
		maxBytes:     15 << 20,
		contentTypes: allowed("image/jpeg", "image/png", "application/pdf"),
	},
	"tenant_document": {
		prefix:       "tenant-documents",
		maxBytes:     25 << 20,
		contentTypes: allowed("application/pdf", "image/jpeg", "image/png"),
	},
	"inspection_reminder": {
		prefix:       "inspections",
		maxBytes:     10 << 20,
		contentTypes: allowed("image/jpeg", "image/png"),
	},
}

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func allowed(values ...string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

type uploadDecision struct {
	ObjectKey string
	MaxBytes  int64
}

func decideUpload(input uploadIntent) (uploadDecision, error) {
	policy, ok := policies[input.AssetKind]
	if !ok {
		return uploadDecision{}, errors.New("asset_kind is not accepted")
	}
	if !safeID.MatchString(input.PropertyID) || !safeID.MatchString(input.RecordID) {
		return uploadDecision{}, errors.New("property_id and record_id must use letters, digits, underscore, or hyphen")
	}
	if input.SizeBytes <= 0 || input.SizeBytes > policy.maxBytes {
		return uploadDecision{}, fmt.Errorf("size_bytes must be between 1 and %d", policy.maxBytes)
	}
	if !policy.contentTypes[input.ContentType] {
		return uploadDecision{}, errors.New("content_type is not accepted for this asset_kind")
	}
	filename := filepath.Base(strings.TrimSpace(input.Filename))
	if filename == "." || filename == "" || filename != input.Filename {
		return uploadDecision{}, errors.New("filename must be a plain file name")
	}
	key := strings.Join([]string{policy.prefix, input.PropertyID, input.RecordID, filename}, "/")
	return uploadDecision{ObjectKey: key, MaxBytes: policy.maxBytes}, nil
}

type uploadIntentResponse struct {
	Method    string    `json:"method"`
	UploadURL string    `json:"upload_url"`
	ObjectKey string    `json:"object_key"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *AssetUploadService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input uploadIntent
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return
	}

	decision, err := decideUpload(input)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	idempotencyKey := intentID(input)
	signed, err := s.storage.PresignPut(r.Context(), s.bucket, decision.ObjectKey, input.ContentType, decision.MaxBytes, idempotencyKey)
	if err != nil {
		status := http.StatusBadGateway
		var apiErr *InfraiError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			status = apiErr.HTTPStatus
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, uploadIntentResponse{
		Method:    http.MethodPut,
		UploadURL: signed.URL,
		ObjectKey: decision.ObjectKey,
		ExpiresAt: s.now().UTC().Add(10 * time.Minute),
	})
}

func intentID(input uploadIntent) string {
	value := strings.Join([]string{input.AssetKind, input.PropertyID, input.RecordID, input.Filename, input.ContentType}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
