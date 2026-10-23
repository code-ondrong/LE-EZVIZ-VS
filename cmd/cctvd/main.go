// Command cctvd runs next to MediaMTX in the Docker image. It passes the HLS streams through to MediaMTX and adds
// what a camera picker needs: a recent picture of every camera (/thumb/<name>.jpg) and whether it is online
// (/status.json). Thumbnails are taken every THUMB_LIVE_INTERVAL while a camera is being watched, otherwise every
// THUMB_IDLE_INTERVAL by starting its stream briefly.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"le-ezviz-vs/client"
	"le-ezviz-vs/logging"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

type camera struct {
	Name   string
	Serial string
}

var (
	listen      = envOr("LISTEN", ":8080")
	hlsURL      = envOr("MEDIAMTX_HLS", "http://127.0.0.1:8888")
	apiURL      = envOr("MEDIAMTX_API", "http://127.0.0.1:9997")
	rtspURL     = envOr("MEDIAMTX_RTSP", "rtsp://127.0.0.1:8554")
	thumbDir    = envOr("THUMB_DIR", "thumbs")
	liveEvery   = envDuration("THUMB_LIVE_INTERVAL", 30*time.Second)
	idleEvery   = envDuration("THUMB_IDLE_INTERVAL", 15*time.Minute)
	statusEvery = envDuration("STATUS_INTERVAL", 5*time.Minute)
	// a failed idle thumbnail (camera offline, EZVIZ error) is retried after this instead of every tick
	retryAfter = 5 * time.Minute
	log        *zap.Logger
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

// parseCameras reads CAMERAS (name:SERIAL,...), the entrypoint has already validated it
func parseCameras(s string) []camera {
	var cams []camera
	for _, entry := range strings.Split(s, ",") {
		name, serial, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if ok && name != "" && serial != "" {
			cams = append(cams, camera{Name: name, Serial: serial})
		}
	}
	return cams
}

type state struct {
	mu       sync.Mutex
	online   map[string]bool // by serial, empty until the first status refresh
	statusAt time.Time
	thumbAt  map[string]time.Time // by camera name
	nextTry  map[string]time.Time // by camera name, set after a failed idle thumbnail
	busy     map[string]bool      // by camera name, a thumbnail is being taken
	cold     bool                 // an idle thumbnail is being taken, they run one at a time
}

func main() {
	if err := logging.CreateLogger(false, "stdout", envOr("LOG_LEVEL", "info")); err != nil {
		panic(err)
	}
	log = logging.Log.With(zap.String("Service", "cctvd"))
	client.SetLogger(log)
	client.TerminalName = envOr("TERMINAL_NAME", "LE-EZ-CCTV")
	cams := parseCameras(os.Getenv("CAMERAS"))
	if len(cams) == 0 {
		log.Fatal("CAMERAS is empty")
	}
	if err := os.MkdirAll(thumbDir, 0755); err != nil {
		log.Fatal("Error creating thumbnail directory", zap.Error(err))
	}
	st := &state{online: map[string]bool{}, thumbAt: map[string]time.Time{}, nextTry: map[string]time.Time{}, busy: map[string]bool{}}
	for _, cam := range cams {
		if fi, err := os.Stat(thumbPath(cam.Name)); err == nil {
			st.thumbAt[cam.Name] = fi.ModTime()
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go refreshStatusLoop(ctx, st)
	go thumbnailLoop(ctx, st, cams)

	target, err := url.Parse(hlsURL)
	if err != nil {
		log.Fatal("Invalid MEDIAMTX_HLS", zap.Error(err))
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	// many viewers poll the same playlists, keep connections to MediaMTX open
	proxy.Transport = &http.Transport{MaxIdleConns: 512, MaxIdleConnsPerHost: 512, IdleConnTimeout: 90 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/status.json", func(w http.ResponseWriter, r *http.Request) { serveStatus(w, st, cams) })
	mux.HandleFunc("/thumb/", func(w http.ResponseWriter, r *http.Request) { serveThumb(w, r, cams) })
	mux.Handle("/", proxy)
	// no write timeout: Low-Latency HLS holds playlist requests open until the next part is ready
	srv := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Info("Listening", zap.String("Address", listen), zap.Int("Cameras", len(cams)))
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal("HTTP server error", zap.Error(err))
	}
}

func thumbPath(name string) string {
	return filepath.Join(thumbDir, name+".jpg")
}

// refreshStatusLoop asks EZVIZ which cameras are online, sharing the session of the camera processes
func refreshStatusLoop(ctx context.Context, st *state) {
	for {
		if err := refreshStatus(st); err != nil {
			log.Warn("Error refreshing camera status", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(statusEvery):
		}
	}
}

func refreshStatus(st *state) error {
	LEZ, err := client.NewLE_EZVIZ_Client(os.Getenv("EZVIZ_EMAIL"), os.Getenv("EZVIZ_PASSWORD"), envOr("EZVIZ_REGION", "Asia"), "00000000000000000000000000000000", client.TerminalName, "shipin7", 15)
	if err != nil {
		return err
	}
	LEZ.LoadFeatureCode("featurecode")
	if err := LEZ.Login("session.json"); err != nil {
		return err
	}
	pageList, err := LEZ.GetPageList()
	if err != nil {
		return err
	}
	if pageList.DeviceInfos == nil {
		return errors.New("page list has no devices")
	}
	online := map[string]bool{}
	for _, d := range *pageList.DeviceInfos {
		online[d.DeviceSerial] = d.Status == 1
	}
	st.mu.Lock()
	st.online, st.statusAt = online, time.Now()
	st.mu.Unlock()
	return nil
}

// readyPaths returns the MediaMTX paths that currently have a stream
func readyPaths() (map[string]bool, error) {
	resp, err := http.Get(apiURL + "/v3/paths/list?itemsPerPage=1000")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("MediaMTX API status %d", resp.StatusCode)
	}
	var body struct {
		Items []struct {
			Name  string `json:"name"`
			Ready bool   `json:"ready"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	ready := map[string]bool{}
	for _, p := range body.Items {
		ready[p.Name] = p.Ready
	}
	return ready, nil
}

func thumbnailLoop(ctx context.Context, st *state, cams []camera) {
	idleSuffix := "-sd"
	if os.Getenv("SUBSTREAM") == "false" {
		idleSuffix = ""
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		ready, err := readyPaths()
		if err != nil {
			log.Debug("MediaMTX API not available", zap.Error(err))
			continue
		}
		now := time.Now()
		for _, cam := range cams {
			live := ""
			if ready[cam.Name] {
				live = cam.Name
			} else if ready[cam.Name+"-sd"] {
				live = cam.Name + "-sd"
			}
			st.mu.Lock()
			online, known := st.online[cam.Serial]
			last := st.thumbAt[cam.Name]
			switch {
			case st.busy[cam.Name]:
			case live != "" && now.Sub(last) >= liveEvery:
				st.busy[cam.Name] = true
				go takeThumbnail(ctx, st, cam, live, false)
			case live == "" && !st.cold && (!known || online) && now.Sub(last) >= idleEvery && now.After(st.nextTry[cam.Name]):
				st.busy[cam.Name], st.cold = true, true
				go takeThumbnail(ctx, st, cam, cam.Name+idleSuffix, true)
			}
			st.mu.Unlock()
		}
	}
}

// takeThumbnail saves one keyframe of the path as a small JPEG. Reading an idle path makes MediaMTX start the camera
func takeThumbnail(ctx context.Context, st *state, cam camera, path string, cold bool) {
	err := snapshot(ctx, path, cam.Name)
	st.mu.Lock()
	defer st.mu.Unlock()
	st.busy[cam.Name] = false
	if cold {
		st.cold = false
	}
	if err != nil {
		st.nextTry[cam.Name] = time.Now().Add(retryAfter)
		log.Warn("Error taking thumbnail", zap.String("Camera", cam.Name), zap.String("Path", path), zap.Error(err))
		return
	}
	st.thumbAt[cam.Name] = time.Now()
	log.Debug("Thumbnail taken", zap.String("Camera", cam.Name), zap.String("Path", path))
}

func snapshot(ctx context.Context, path, name string) error {
	// a cold camera needs up to 30s to start, then a keyframe
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	tmp := filepath.Join(thumbDir, name+".tmp.jpg")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin",
		"-rtsp_transport", "tcp", "-skip_frame", "nokey", "-i", rtspURL+"/"+path,
		"-frames:v", "1", "-vf", "scale=480:-2", "-q:v", "5", "-update", "1", "-y", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return os.Rename(tmp, thumbPath(name))
}

type cameraStatus struct {
	Name string `json:"name"`
	// null until EZVIZ has been asked
	Online *bool `json:"online"`
	// versioned so browsers fetch it again only when it changed
	Thumb   string     `json:"thumb,omitempty"`
	ThumbAt *time.Time `json:"thumbAt,omitempty"`
}

func serveStatus(w http.ResponseWriter, st *state, cams []camera) {
	st.mu.Lock()
	out := struct {
		Cameras  []cameraStatus `json:"cameras"`
		StatusAt *time.Time     `json:"statusAt"`
	}{Cameras: make([]cameraStatus, 0, len(cams))}
	if !st.statusAt.IsZero() {
		at := st.statusAt
		out.StatusAt = &at
	}
	for _, cam := range cams {
		cs := cameraStatus{Name: cam.Name}
		if online, ok := st.online[cam.Serial]; ok {
			cs.Online = &online
		}
		if at, ok := st.thumbAt[cam.Name]; ok {
			cs.Thumb = fmt.Sprintf("/thumb/%s.jpg?v=%d", cam.Name, at.Unix())
			cs.ThumbAt = &at
		}
		out.Cameras = append(out.Cameras, cs)
	}
	st.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=15")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(out)
}

func serveThumb(w http.ResponseWriter, r *http.Request, cams []camera) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/thumb/"), ".jpg")
	for _, cam := range cams {
		if cam.Name == name {
			w.Header().Set("Cache-Control", "public, max-age=300")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			http.ServeFile(w, r, thumbPath(name))
			return
		}
	}
	http.NotFound(w, r)
}
