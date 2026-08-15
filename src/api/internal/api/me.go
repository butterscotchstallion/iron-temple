package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/auth"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Avatar limits.
//
// There is no object store and no volume in this deployment — only the tenant
// database (see deploy/) — so avatars live in Postgres. That is fine at this
// size and only at this size, hence a cap small enough that a row stays cheap
// to read on every profile render.
const (
	maxAvatarBytes = 256 << 10 // 256 KiB
	// maxAvatarDim rejects images that are large in pixels rather than bytes —
	// a 20000x20000 PNG of flat colour compresses to almost nothing but costs
	// gigabytes to decode. There is no resizer in the standard library and no
	// new dependency is available here, so oversized images are refused rather
	// than scaled down.
	maxAvatarDim = 1024
)

// avatarColorPattern accepts a CSS hex colour, the only form the UI produces.
// Anything else is refused rather than escaped, because this value is
// interpolated into a style attribute.
var avatarColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func (s *Server) getMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	writeJSON(w, http.StatusOK, s.userDTO(ctx, store.GetUserRow{
		ID:               u.ID,
		Username:         u.Username,
		DisplayName:      u.DisplayName,
		AvatarColor:      u.AvatarColor,
		IsAdmin:          u.IsAdmin,
		CurrentProgramID: u.CurrentProgramID,
	}))
}

type updateProfileRequest struct {
	DisplayName      *string `json:"displayName"`
	AvatarColor      *string `json:"avatarColor"`
	CurrentProgramID *int32  `json:"currentProgramId"`
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	params := store.UpdateUserProfileParams{ID: u.ID}
	if req.DisplayName != nil {
		name := strings.TrimSpace(*req.DisplayName)
		if name == "" || utf8.RuneCountInString(name) > maxDisplayName {
			badRequest(w, "displayName must be between 1 and 64 characters")
			return
		}
		params.DisplayName = &name
	}
	if req.AvatarColor != nil {
		colour := strings.TrimSpace(*req.AvatarColor)
		// Empty means "go back to the colour derived from my id".
		if colour != "" && !avatarColorPattern.MatchString(colour) {
			badRequest(w, "avatarColor must be a hex colour such as #b026ff")
			return
		}
		params.AvatarColor = &colour
	}
	if req.CurrentProgramID != nil {
		// Checked here rather than left to the foreign key: an unknown id is the
		// caller's mistake, and a constraint violation surfacing from the UPDATE
		// would be indistinguishable from a real failure and served as a 500.
		if _, err := s.q.GetProgram(ctx, *req.CurrentProgramID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				badRequest(w, "currentProgramId must be an existing program")
				return
			}
			internalError(w)
			return
		}
		params.CurrentProgramID = req.CurrentProgramID
	}

	updated, err := s.q.UpdateUserProfile(ctx, params)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "user not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, s.userDTO(ctx, store.GetUserRow{
		ID:               updated.ID,
		Username:         updated.Username,
		DisplayName:      updated.DisplayName,
		AvatarColor:      updated.AvatarColor,
		IsAdmin:          updated.IsAdmin,
		CurrentProgramID: updated.CurrentProgramID,
	}))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// changePassword requires the current password even though the caller is
// already authenticated: it is what stops a borrowed session — an unlocked
// laptop, a stolen cookie — from being upgraded into permanent ownership of the
// account.
func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if msg, ok := validatePassword(req.NewPassword); !ok {
		badRequest(w, msg)
		return
	}

	user, err := s.q.GetUserForLogin(ctx, u.Username)
	if err != nil {
		internalError(w)
		return
	}
	if ok, _ := s.hasher.Verify(req.CurrentPassword, user.PasswordHash); !ok {
		unauthorized(w, "current password is incorrect")
		return
	}

	hash, err := s.hasher.Hash(req.NewPassword)
	if err != nil {
		internalError(w)
		return
	}
	if _, err := s.q.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{
		PasswordHash: hash, ID: u.ID,
	}); err != nil {
		internalError(w)
		return
	}

	// Revoke every other login. Changing a password is how someone responds to
	// believing it was compromised; leaving the attacker's cookie valid would
	// make the whole gesture ceremonial. The current session survives so the
	// user is not signed out of the tab they just used.
	if _, err := s.q.DeleteUserSessionsExcept(ctx, store.DeleteUserSessionsExceptParams{
		UserID: u.ID, TokenHash: u.tokenHash,
	}); err != nil {
		internalError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// uploadAvatar accepts a PNG or JPEG and stores a re-encoded copy.
func (s *Server) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)

	// Cap the body before anything reads it, so an oversized upload is refused
	// at the socket rather than buffered into memory first.
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes)

	file, _, err := r.FormFile("avatar")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "too_large",
				fmt.Sprintf("avatar must be at most %d KB", maxAvatarBytes>>10))
			return
		}
		badRequest(w, "expected a multipart form with an 'avatar' file field")
		return
	}
	defer func() { _ = file.Close() }()

	encoded, mime, err := decodeAvatar(file)
	if err != nil {
		badRequest(w, err.Error())
		return
	}

	sum := sha256.Sum256(encoded)
	etag := hex.EncodeToString(sum[:])

	if err := s.q.UpsertUserAvatar(ctx, store.UpsertUserAvatarParams{
		UserID: u.ID, Mime: mime, Bytes: encoded, Etag: etag,
	}); err != nil {
		internalError(w)
		return
	}

	writeJSON(w, http.StatusOK, avatarDTO{Etag: etag})
}

// decodeAvatar validates an uploaded image and returns bytes safe to store.
//
// The declared Content-Type is not consulted: it is client-supplied and proves
// nothing. Decoding the image is the actual check, and re-encoding the decoded
// pixels is what makes the stored bytes safe — it drops EXIF (which can carry
// GPS coordinates from a phone photo) and any data appended past the end of the
// image, so what gets served back is a file this server produced rather than
// one a stranger handed us.
func decodeAvatar(r io.Reader) (encoded []byte, mime string, err error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", errors.New("avatar must be a valid PNG or JPEG image")
	}

	bounds := img.Bounds()
	if bounds.Dx() > maxAvatarDim || bounds.Dy() > maxAvatarDim {
		return nil, "", fmt.Errorf("avatar must be at most %dx%d pixels", maxAvatarDim, maxAvatarDim)
	}

	var buf bytes.Buffer
	switch format {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", errors.New("could not re-encode the image")
		}
		return buf.Bytes(), "image/png", nil
	case "jpeg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
			return nil, "", errors.New("could not re-encode the image")
		}
		return buf.Bytes(), "image/jpeg", nil
	default:
		// image.Decode only knows the formats whose packages are imported, so
		// reaching here means one was added without extending this switch.
		return nil, "", errors.New("avatar must be a PNG or JPEG image")
	}
}

func (s *Server) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	if _, err := s.q.DeleteUserAvatar(r.Context(), userFrom(r.Context()).ID); err != nil {
		internalError(w)
		return
	}
	// Idempotent: deleting an avatar that was never uploaded is a success, not
	// a 404 — the caller's desired state is reached either way.
	w.WriteHeader(http.StatusNoContent)
}

// getUserAvatar serves an avatar's bytes.
//
// Unauthenticated on purpose: this is an <img> src, and browsers cannot attach
// credentials to one without CORS gymnastics. What it exposes is a picture the
// user chose to represent themselves, keyed by a numeric id — no more than a
// forum avatar. Everything that is actually private stays behind requireUser.
func (s *Server) getUserAvatar(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "userId")
	if !ok {
		notFound(w, "avatar not found")
		return
	}

	avatar, err := s.q.GetUserAvatar(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "avatar not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	etag := `"` + avatar.Etag + `"`
	w.Header().Set("ETag", etag)
	// Private: an avatar is per-user, so a shared cache must not hold it. The
	// UI appends the etag to the URL as a cache-buster, so must-revalidate
	// costs one conditional request and gets an instant update after a change.
	w.Header().Set("Cache-Control", "private, max-age=0, must-revalidate")

	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, avatar.Etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", avatar.Mime)
	w.Header().Set("Content-Length", fmt.Sprint(len(avatar.Bytes)))
	// Stored bytes are re-encoded by decodeAvatar, but say it anyway: nothing
	// here should ever be sniffed into an executable type.
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(avatar.Bytes)
}

// userDTO assembles the profile payload, looking up whether an avatar exists
// without pulling its bytes.
func (s *Server) userDTO(ctx context.Context, u store.GetUserRow) userDTO {
	dto := userDTO{
		ID:               u.ID,
		Username:         u.Username,
		DisplayName:      u.DisplayName,
		AvatarColor:      u.AvatarColor,
		IsAdmin:          u.IsAdmin,
		CurrentProgramID: u.CurrentProgramID,
	}
	etag, err := s.q.GetUserAvatarEtag(ctx, u.ID)
	if err == nil {
		dto.HasAvatar = true
		dto.AvatarEtag = etag
	}
	return dto
}

// compile-time guard: auth.Hasher must keep satisfying what the handlers use.
var _ interface {
	Hash(string) (string, error)
	Verify(string, string) (bool, bool)
} = auth.PBKDF2Hasher{}
