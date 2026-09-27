package filebrowser

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/marcopiovanello/yt-dlp-web-ui/v3/server/config"
	"github.com/marcopiovanello/yt-dlp-web-ui/v3/server/internal"
)

/*
	File based operation handlers (should be moved to rest/handlers.go) or in
	a entirely self-contained package
*/

var (
	videoRe = regexp.MustCompile(`(?i)\.(mov|mp4|webm|mkv|mvk|avi|flv|m4v|mpg|mpeg|ts)$`)
	imageRe = regexp.MustCompile(`(?i)\.(jpg|jpeg|png|webp|gif|avif|bmp)$`)
	audioRe = regexp.MustCompile(`(?i)\.(mp3|m4a|opus|ogg|flac|wav|aac)$`)
)

func isVideo(d fs.DirEntry) bool {
	return videoRe.MatchString(d.Name())
}

func isImage(d fs.DirEntry) bool {
	return imageRe.MatchString(d.Name())
}

func isAudio(d fs.DirEntry) bool {
	return audioRe.MatchString(d.Name())
}

func isValidEntry(d fs.DirEntry) bool {
	return !strings.HasPrefix(d.Name(), ".") &&
		!strings.HasSuffix(d.Name(), ".part") &&
		!strings.HasSuffix(d.Name(), ".ytdl")
}

type DirectoryEntry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"modTime"`
	IsVideo     bool      `json:"isVideo"`
	IsImage     bool      `json:"isImage"`
	IsAudio     bool      `json:"isAudio"`
	IsDirectory bool      `json:"isDirectory"`
}

func walkDir(root string) (*[]DirectoryEntry, error) {
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var files []DirectoryEntry

	for _, d := range dirs {
		if !isValidEntry(d) {
			continue
		}

		path := filepath.Join(root, d.Name())

		info, err := d.Info()
		if err != nil {
			return nil, err
		}

		files = append(files, DirectoryEntry{
			Path:        path,
			Name:        d.Name(),
			Size:        info.Size(),
			IsVideo:     isVideo(d),
			IsImage:     isImage(d),
			IsAudio:     isAudio(d),
			IsDirectory: d.IsDir(),
			ModTime:     info.ModTime(),
		})
	}

	return &files, err
}

type ListRequest struct {
	SubDir  string `json:"subdir"`
	OrderBy string `json:"orderBy"`
}

func ListDownloaded(w http.ResponseWriter, r *http.Request) {
	root := config.Instance().DownloadPath
	req := new(ListRequest)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	files, err := walkDir(filepath.Join(root, req.SubDir))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.OrderBy == "modtime" {
		sort.SliceStable(*files, func(i, j int) bool {
			return (*files)[i].ModTime.After((*files)[j].ModTime)
		})
	}

	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(files); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type DeleteRequest = DirectoryEntry

func DeleteFile(w http.ResponseWriter, r *http.Request) {
	req := new(DeleteRequest)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.Remove(req.Path); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode("ok")
}

func SendFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		http.Error(w, "inexistent path", http.StatusBadRequest)
		return
	}

	decoded, err := decodeID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filename := decoded

	root := config.Instance().DownloadPath

	if strings.Contains(filepath.Dir(filepath.Clean(filename)), filepath.Clean(root)) {
		http.ServeFile(w, r, filename)
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
}

func DownloadFile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if id == "" {
		http.Error(w, "inexistent path", http.StatusBadRequest)
		return
	}

	decoded, err := decodeID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filename := decoded

	root := config.Instance().DownloadPath

	if strings.Contains(filepath.Dir(filepath.Clean(filename)), filepath.Clean(root)) {
		w.Header().Add("Content-Disposition", "inline; filename=\""+filepath.Base(filename)+"\"")
		w.Header().Set("Content-Type", "application/octet-stream")

		fd, err := os.Open(filename)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		io.Copy(w, fd)
		return
	}

	w.WriteHeader(http.StatusUnauthorized)
}

func BulkDownload(mdb *internal.MemoryDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ps := slices.DeleteFunc(*mdb.All(), func(e internal.ProcessResponse) bool {
			return e.Progress.Status != internal.StatusCompleted
		})

		if len(ps) == 0 {
			return
		}

		zipWriter := zip.NewWriter(w)

		w.Header().Add(
			"Content-Disposition",
			"inline; filename=download-archive-"+time.Now().Format(time.RFC3339)+".zip",
		)
		w.Header().Set("Content-Type", "application/zip")

		for _, p := range ps {
			wr, err := zipWriter.Create(filepath.Base(p.Output.SavedFilePath))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			fd, err := os.Open(p.Output.SavedFilePath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if _, err := io.Copy(wr, fd); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if err := zipWriter.Close(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// decodeID decodes the path id used by the filebrowser routes. The frontend
// sends base64url without padding, older links use standard base64 with
// padding: both are accepted, as well as percent-encoded input.
func decodeID(id string) (string, error) {
	if unescaped, err := url.QueryUnescape(id); err == nil {
		id = unescaped
	}

	for _, encoding := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if decoded, err := encoding.DecodeString(id); err == nil {
			return string(decoded), nil
		}
	}

	return "", errors.New("invalid path encoding")
}

// Thumbnail sends a small JPEG preview of a downloaded file: a frame grabbed
// with ffmpeg for videos, the embedded cover art for audio files, or the file
// itself when it already is an image. Generated previews are cached next to the
// local database, so the same file is only decoded once.
func Thumbnail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	decoded, err := decodeID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	path := decoded

	if !withinDownloadPath(path) {
		http.Error(w, "file is outside of the download directory", http.StatusForbidden)
		return
	}

	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.Error(w, "no preview available", http.StatusNotFound)
		return
	}

	ext := filepath.Ext(path)

	if !imageRe.MatchString(ext) && !videoRe.MatchString(ext) && !audioRe.MatchString(ext) {
		http.Error(w, "no preview available for this file type", http.StatusNotFound)
		return
	}

	thumb, err := generateThumbnail(path, info)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "private, max-age=86400")

	http.ServeFile(w, r, thumb)
}

// withinDownloadPath reports whether path resolves inside the configured
// download directory.
func withinDownloadPath(path string) bool {
	root, err := filepath.Abs(config.Instance().DownloadPath)
	if err != nil {
		return false
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	return abs == root || strings.HasPrefix(abs, root+string(os.PathSeparator))
}

// generateThumbnail extracts the first frame of a media file into the cache
// directory and returns the path of the cached image.
func generateThumbnail(path string, info os.FileInfo) (string, error) {
	dir := filepath.Dir(config.Instance().LocalDatabasePath)
	if dir == "" || dir == "." {
		dir = os.TempDir()
	}

	cacheDir := filepath.Join(dir, "thumbs")

	sum := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().Unix())))
	target := filepath.Join(cacheDir, fmt.Sprintf("%x.jpg", sum))

	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}

	tmp := target + ".tmp"

	// try a keyframe a few seconds in, then the very beginning of the file
	for _, seek := range []string{"3", "0"} {
		cmd := exec.Command(
			"ffmpeg",
			"-y",
			"-v", "error",
			"-ss", seek,
			"-i", path,
			"-frames:v", "1",
			"-vf", "scale=480:-2",
			"-q:v", "6",
			"-f", "mjpeg",
			tmp,
		)

		if err := cmd.Run(); err != nil {
			continue
		}

		if _, err := os.Stat(tmp); err != nil {
			continue
		}

		if err := os.Rename(tmp, target); err != nil {
			return "", err
		}

		return target, nil
	}

	os.Remove(tmp)

	return "", errors.New("no frame could be extracted from this file")
}
