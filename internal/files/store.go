package files

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/access"
	"lmsv3/internal/egress"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

var ErrForbidden = errors.New("FORBIDDEN")
var ErrConflict = errors.New("CONFLICT")
var ErrInvalid = errors.New("INVALID_INPUT")
var ErrNotFound = errors.New("NOT_FOUND")
var ErrUnavailable = errors.New("UNAVAILABLE")

var idempotencyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,80}$`)

type Asset struct {
	ID                string     `json:"id"`
	ClassID           string     `json:"class_id"`
	OwnerUserID       string     `json:"owner_user_id"`
	Purpose           string     `json:"purpose"`
	OriginalName      string     `json:"original_name"`
	DeclaredMediaType string     `json:"declared_media_type"`
	DetectedMediaType string     `json:"detected_media_type"`
	ByteSize          int64      `json:"byte_size"`
	SHA256            string     `json:"sha256"`
	State             string     `json:"state"`
	ScanStatus        string     `json:"scan_status"`
	ScanCode          string     `json:"scan_code"`
	CreatedAt         time.Time  `json:"created_at"`
	AvailableAt       *time.Time `json:"available_at,omitempty"`
	objectKey         string
}

type Upload struct {
	ClassID      string
	OwnerUserID  string
	Purpose      string
	OriginalName string
	DeclaredType string
	Data         []byte
	requestScope string
}

type Grant struct {
	Token     string    `json:"token"`
	AssetID   string    `json:"asset_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store struct {
	pool     *pgxpool.Pool
	objects  ObjectStore
	scanner  Scanner
	grantTTL time.Duration
	now      func() time.Time
	resolve  func(context.Context, pgx.Tx, string) (identity.Actor, error)
	remote   RemoteFetcher
}

type RemoteFetcher interface {
	Fetch(context.Context, string) (egress.Result, error)
}

type Option func(*Store) error

func WithRemoteFetcher(fetcher RemoteFetcher) Option {
	return func(store *Store) error {
		if fetcher == nil {
			return errors.New("REMOTE_FETCHER_REJECTED")
		}
		store.remote = fetcher
		return nil
	}
}

func New(pool *pgxpool.Pool, objects ObjectStore, scanner Scanner, grantTTL time.Duration, options ...Option) (*Store, error) {
	if pool == nil || objects == nil || scanner == nil || grantTTL < time.Second || grantTTL > 5*time.Minute {
		return nil, errors.New("FILE_STORE_CONFIG_REJECTED")
	}
	store := &Store{pool: pool, objects: objects, scanner: scanner, grantTTL: grantTTL, now: time.Now, resolve: identity.ResolveBridge}
	for _, option := range options {
		if option == nil || option(store) != nil {
			return nil, errors.New("FILE_STORE_CONFIG_REJECTED")
		}
	}
	return store, nil
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func digest(data []byte) string {
	value := sha256.Sum256(data)
	return hex.EncodeToString(value[:])
}

func normalizeMetadata(input Upload) (Upload, error) {
	input.Purpose = strings.ToUpper(strings.TrimSpace(input.Purpose))
	input.OriginalName = strings.TrimSpace(input.OriginalName)
	input.DeclaredType = strings.ToLower(strings.TrimSpace(input.DeclaredType))
	if input.ClassID == "" || input.OwnerUserID == "" || (input.Purpose != "MATERIAL" && input.Purpose != "EVIDENCE") {
		return input, ErrInvalid
	}
	if len(input.OriginalName) > 180 || input.OriginalName != filepath.Base(input.OriginalName) || strings.ContainsAny(input.OriginalName, `/\`) {
		return input, ErrInvalid
	}
	for _, character := range input.OriginalName {
		if unicode.IsControl(character) {
			return input, ErrInvalid
		}
	}
	allowed := map[string]bool{"application/pdf": true, "image/png": true, "image/jpeg": true, "text/plain": true}
	if !allowed[input.DeclaredType] {
		return input, ErrInvalid
	}
	return input, nil
}

func normalizeUpload(input Upload) (Upload, string, error) {
	input, err := normalizeMetadata(input)
	if err != nil || len(input.Data) < 1 || len(input.Data) > MaxBytes {
		return input, "", ErrInvalid
	}
	detected := strings.ToLower(strings.TrimSpace(strings.Split(http.DetectContentType(input.Data), ";")[0]))
	if detected != input.DeclaredType {
		return input, "", ErrInvalid
	}
	return input, detected, nil
}

func (s *Store) classContext(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID, action, purpose, ownerID string) error {
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM v3.classes WHERE id=$1 FOR SHARE", classID).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	var closed bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.class_access WHERE class_id=$1 AND closed_at IS NOT NULL)", classID).Scan(&closed); err != nil {
		return err
	}
	if closed && state == "PUBLISHED" {
		state = "CLOSED"
	}
	rows, err := tx.Query(ctx, `SELECT r.role FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL)`, classID, actor.ID)
	if err != nil {
		return err
	}
	roles := []string{}
	for rows.Next() {
		var role string
		if err = rows.Scan(&role); err != nil {
			rows.Close()
			return err
		}
		roles = append(roles, role)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if action == "upload" && ownerID != actor.ID {
		var ownedHere bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.enrollments WHERE class_id=$1 AND user_id=$2 AND status='ACTIVE')", classID, ownerID).Scan(&ownedHere); err != nil {
			return err
		}
		if !ownedHere {
			return ErrForbidden
		}
	}
	if !access.AllowsFile(actor.Admin, roles, state, action, purpose, ownerID == actor.ID) {
		return ErrForbidden
	}
	return nil
}

func (s *Store) asset(ctx context.Context, tx pgx.Tx, id string) (Asset, error) {
	var asset Asset
	err := tx.QueryRow(ctx, `SELECT id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_at,available_at
 FROM v3.file_assets WHERE id=$1`, id).Scan(&asset.ID, &asset.ClassID, &asset.OwnerUserID, &asset.Purpose, &asset.objectKey, &asset.OriginalName, &asset.DeclaredMediaType, &asset.DetectedMediaType, &asset.ByteSize, &asset.SHA256, &asset.State, &asset.ScanStatus, &asset.ScanCode, &asset.CreatedAt, &asset.AvailableAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return asset, ErrNotFound
	}
	return asset, err
}

func audit(ctx context.Context, tx pgx.Tx, actor, classID, action, requestID string) error {
	event, err := randomHex(16)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,$3,$4,$5)", event, actor, classID, action, requestID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", event)
	return err
}

func (s *Store) Upload(ctx context.Context, token, key, requestID string, input Upload) (Asset, bool, error) {
	if !idempotencyPattern.MatchString(key) || requestID == "" {
		return Asset{}, false, ErrInvalid
	}
	input, detected, err := normalizeUpload(input)
	if err != nil {
		return Asset{}, false, err
	}
	contentHash := digest(input.Data)
	requestHash := digest([]byte(strings.Join([]string{input.ClassID, input.OwnerUserID, input.Purpose, input.OriginalName, input.DeclaredType, contentHash, input.requestScope}, "\x00")))
	var prior Asset
	priorFound := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		if err = s.classContext(ctx, tx, actor, input.ClassID, "upload", input.Purpose, input.OwnerUserID); err != nil {
			return err
		}
		var priorHash string
		var priorJSON []byte
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.file_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&priorHash, &priorJSON)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if priorHash != requestHash || json.Unmarshal(priorJSON, &prior) != nil {
			return ErrConflict
		}
		priorFound = true
		return nil
	})
	if err != nil {
		return Asset{}, false, err
	}
	if priorFound {
		return prior, true, nil
	}
	// A client-visible checksum is not a storage locator. Keeping these values
	// independent prevents a checksum disclosure from revealing a private path.
	objectKey, err := randomHex(32)
	if err != nil {
		return Asset{}, false, err
	}
	if err = s.objects.PutQuarantine(objectKey, contentHash, input.Data); err != nil {
		return Asset{}, false, ErrUnavailable
	}
	state, scanStatus, scanCode := "AVAILABLE", "CLEAN", "CLEAN"
	scanErr := s.scanner.Scan(ctx, input.Data)
	if errors.Is(scanErr, ErrThreat) {
		state, scanStatus, scanCode = "REJECTED", "THREAT", "THREAT_DETECTED"
	} else if scanErr != nil {
		state, scanStatus, scanCode = "QUARANTINED", "UNAVAILABLE", "SCAN_UNAVAILABLE"
	} else if err = s.objects.Promote(objectKey, contentHash); err != nil {
		state, scanStatus, scanCode = "QUARANTINED", "UNAVAILABLE", "STORAGE_PROMOTION_FAILED"
	}
	assetID, err := randomHex(16)
	if err != nil {
		return Asset{}, false, err
	}
	var result Asset
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		if err = s.classContext(ctx, tx, actor, input.ClassID, "upload", input.Purpose, input.OwnerUserID); err != nil {
			return err
		}
		var priorHash string
		var priorJSON []byte
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.file_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&priorHash, &priorJSON)
		if err == nil {
			if priorHash != requestHash || json.Unmarshal(priorJSON, &result) != nil {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var available any
		if state == "AVAILABLE" {
			available = s.now().UTC()
		}
		result = Asset{ID: assetID, ClassID: input.ClassID, OwnerUserID: input.OwnerUserID, Purpose: input.Purpose, OriginalName: input.OriginalName, DeclaredMediaType: input.DeclaredType, DetectedMediaType: detected, ByteSize: int64(len(input.Data)), SHA256: contentHash, State: state, ScanStatus: scanStatus, ScanCode: scanCode, objectKey: objectKey}
		if _, err = tx.Exec(ctx, `INSERT INTO v3.file_assets(id,class_id,owner_user_id,purpose,object_key,original_name,declared_media_type,detected_media_type,byte_size,sha256,state,scan_status,scan_code,created_by,available_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, result.ID, result.ClassID, result.OwnerUserID, result.Purpose, objectKey, result.OriginalName, result.DeclaredMediaType, result.DetectedMediaType, result.ByteSize, result.SHA256, result.State, result.ScanStatus, result.ScanCode, actor.ID, available); err != nil {
			return err
		}
		if err = audit(ctx, tx, actor.ID, result.ClassID, "file.upload."+strings.ToLower(result.State), requestID); err != nil {
			return err
		}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3.file_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb)", actor.ID, key, requestHash, string(response))
		return err
	})
	if err != nil {
		return Asset{}, false, err
	}
	return result, replayed, nil
}

func (s *Store) Metadata(ctx context.Context, token, assetID string) (Asset, error) {
	var result Asset
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		result, err = s.asset(ctx, tx, assetID)
		if err != nil {
			return err
		}
		return s.classContext(ctx, tx, actor, result.ClassID, "metadata", result.Purpose, result.OwnerUserID)
	})
	return result, err
}

func (s *Store) Rescan(ctx context.Context, token, assetID, requestID string) (Asset, error) {
	var asset Asset
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		asset, err = s.asset(ctx, tx, assetID)
		if err != nil {
			return err
		}
		if asset.State != "QUARANTINED" {
			return ErrConflict
		}
		return s.classContext(ctx, tx, actor, asset.ClassID, "scan", asset.Purpose, asset.OwnerUserID)
	})
	if err != nil {
		return Asset{}, err
	}
	data, err := s.objects.ReadQuarantine(asset.objectKey, asset.SHA256)
	if err != nil || int64(len(data)) != asset.ByteSize || digest(data) != asset.SHA256 {
		return Asset{}, ErrUnavailable
	}
	scanErr := s.scanner.Scan(ctx, data)
	nextState, nextStatus, code := "AVAILABLE", "CLEAN", "CLEAN"
	if errors.Is(scanErr, ErrThreat) {
		nextState, nextStatus, code = "REJECTED", "THREAT", "THREAT_DETECTED"
	} else if scanErr != nil {
		return asset, ErrUnavailable
	} else if err = s.objects.Promote(asset.objectKey, asset.SHA256); err != nil {
		return asset, ErrUnavailable
	}
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		current, err := s.asset(ctx, tx, assetID)
		if err != nil {
			return err
		}
		if current.State != "QUARANTINED" || current.SHA256 != asset.SHA256 {
			return ErrConflict
		}
		if err = s.classContext(ctx, tx, actor, current.ClassID, "scan", current.Purpose, current.OwnerUserID); err != nil {
			return err
		}
		var available any
		if nextState == "AVAILABLE" {
			available = s.now().UTC()
		}
		if _, err = tx.Exec(ctx, "UPDATE v3.file_assets SET state=$2,scan_status=$3,scan_code=$4,available_at=$5 WHERE id=$1 AND state='QUARANTINED'", assetID, nextState, nextStatus, code, available); err != nil {
			return err
		}
		return audit(ctx, tx, actor.ID, current.ClassID, "file.rescan."+strings.ToLower(nextState), requestID)
	})
	if err != nil {
		return Asset{}, err
	}
	return s.Metadata(ctx, token, assetID)
}

func (s *Store) Grant(ctx context.Context, token, assetID, requestID string) (Grant, error) {
	raw, err := randomHex(32)
	if err != nil {
		return Grant{}, err
	}
	result := Grant{Token: raw, AssetID: assetID, ExpiresAt: s.now().UTC().Add(s.grantTTL)}
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, token)
		if err != nil {
			return err
		}
		asset, err := s.asset(ctx, tx, assetID)
		if err != nil {
			return err
		}
		if asset.State != "AVAILABLE" || asset.ScanStatus != "CLEAN" {
			return ErrConflict
		}
		if err = s.classContext(ctx, tx, actor, asset.ClassID, "grant", asset.Purpose, asset.OwnerUserID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.file_download_grants(token_hash,asset_id,user_id,expires_at) VALUES($1,$2,$3,$4)", digest([]byte(raw)), asset.ID, actor.ID, result.ExpiresAt); err != nil {
			return err
		}
		return audit(ctx, tx, actor.ID, asset.ClassID, "file.download_grant", requestID)
	})
	return result, err
}

func (s *Store) Download(ctx context.Context, session, grant, requestID string) (Asset, []byte, error) {
	if len(grant) != 64 || !objectKeyPattern.MatchString(grant) {
		return Asset{}, nil, ErrNotFound
	}
	grantHash := digest([]byte(grant))
	var authorized Asset
	var actorID string
	check := func(tx pgx.Tx) error {
		actor, err := s.resolve(ctx, tx, session)
		if err != nil {
			return err
		}
		actorID = actor.ID
		var assetID, target string
		var expires time.Time
		if err = tx.QueryRow(ctx, "SELECT asset_id,user_id,expires_at FROM v3.file_download_grants WHERE token_hash=$1", grantHash).Scan(&assetID, &target, &expires); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if target != actor.ID {
			return ErrForbidden
		}
		if !s.now().UTC().Before(expires) {
			return ErrNotFound
		}
		authorized, err = s.asset(ctx, tx, assetID)
		if err != nil {
			return err
		}
		if authorized.State != "AVAILABLE" || authorized.ScanStatus != "CLEAN" {
			return ErrConflict
		}
		return s.classContext(ctx, tx, actor, authorized.ClassID, "download", authorized.Purpose, authorized.OwnerUserID)
	}
	if err := transaction.Within(ctx, s.pool, check); err != nil {
		return Asset{}, nil, err
	}
	data, err := s.objects.Read(authorized.objectKey, authorized.SHA256)
	if err != nil || int64(len(data)) != authorized.ByteSize || digest(data) != authorized.SHA256 {
		return Asset{}, nil, ErrUnavailable
	}
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		if err := check(tx); err != nil {
			return err
		}
		event, err := randomHex(16)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.file_access_events(id,asset_id,user_id,grant_hash,outcome) VALUES($1,$2,$3,$4,'DELIVERED')", event, authorized.ID, actorID, grantHash); err != nil {
			return err
		}
		return audit(ctx, tx, actorID, authorized.ClassID, "file.download", requestID)
	})
	return authorized, data, err
}
