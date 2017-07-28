package vidplayer

import (
	"context"
	"errors"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"

	"strings"

	"time"

	"github.com/ericxtang/m3u8"
	"github.com/golang/glog"
	"github.com/livepeer/lpms/stream"
	joy4rtmp "github.com/nareix/joy4/format/rtmp"
)

var ErrNotFound = errors.New("NotFound")
var ErrRTMP = errors.New("RTMP Error")
var PlaylistWaittime = 6 * time.Second

//VidPlayer is the module that handles playing video. For now we only support RTMP and HLS play.
type VidPlayer struct {
	RtmpServer      *joy4rtmp.Server
	rtmpPlayHandler func(url *url.URL) (stream.Stream, error)
	VodPath         string
}

func defaultRtmpPlayHandler(url *url.URL) (stream.Stream, error) { return nil, ErrNotFound }

//NewVidPlayer creates a new video player
func NewVidPlayer(rtmpS *joy4rtmp.Server, vodPath string) *VidPlayer {
	player := &VidPlayer{RtmpServer: rtmpS, VodPath: vodPath, rtmpPlayHandler: defaultRtmpPlayHandler}
	rtmpS.HandlePlay = player.rtmpServerHandlePlay()
	return player
}

//HandleRTMPPlay is the handler when there is a RTMP request for a video. The source should write
//into the MuxCloser. The easiest way is through avutil.Copy.
func (s *VidPlayer) HandleRTMPPlay(getStream func(url *url.URL) (stream.Stream, error)) error {
	s.rtmpPlayHandler = getStream
	return nil
}

func (s *VidPlayer) rtmpServerHandlePlay() func(conn *joy4rtmp.Conn) {
	return func(conn *joy4rtmp.Conn) {
		glog.Infof("LPMS got RTMP request @ %v", conn.URL)

		src, err := s.rtmpPlayHandler(conn.URL)
		if err != nil {
			glog.Errorf("Error getting stream: %v", err)
			return
		}

		err = src.ReadRTMPFromStream(context.Background(), conn)
		if err != nil {
			glog.Errorf("Error copying RTMP stream: %v", err)
			return
		}
	}
}

//HandleHLSPlay is the handler when there is a HLS video request.  It supports both VOD and live streaming.
func (s *VidPlayer) HandleHLSPlay(
	getMasterPlaylist func(url *url.URL) (*m3u8.MasterPlaylist, error),
	getMediaPlaylist func(url *url.URL) (*m3u8.MediaPlaylist, error),
	getSegment func(url *url.URL) ([]byte, error)) {

	http.HandleFunc("/stream/", func(w http.ResponseWriter, r *http.Request) {
		handleLive(w, r, getMasterPlaylist, getMediaPlaylist, getSegment)
	})

	http.HandleFunc("/vod/", func(w http.ResponseWriter, r *http.Request) {
		handleVOD(r.URL, s.VodPath, w)
	})
}

func handleLive(w http.ResponseWriter, r *http.Request,
	getMasterPlaylist func(url *url.URL) (*m3u8.MasterPlaylist, error),
	getMediaPlaylist func(url *url.URL) (*m3u8.MediaPlaylist, error),
	getSegment func(url *url.URL) ([]byte, error)) {

	glog.Infof("LPMS got HTTP request @ %v", r.URL.Path)

	if !strings.HasSuffix(r.URL.Path, ".m3u8") && !strings.HasSuffix(r.URL.Path, ".ts") {
		http.Error(w, "LPMS only accepts HLS requests over HTTP (m3u8, ts).", 500)
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "max-age=5")

	if strings.HasSuffix(r.URL.Path, ".m3u8") {
		w.Header().Set("Content-Type", "application/x-mpegURL")
		//First, assume it's the master playlist
		var masterPl *m3u8.MasterPlaylist
		var mediaPl *m3u8.MediaPlaylist
		masterPl, err := getMasterPlaylist(r.URL)
		if masterPl == nil || err != nil {
			//Now try the media playlist
			mediaPl, err = getMediaPlaylist(r.URL)
			if err != nil {
				http.Error(w, "Error getting HLS playlist", 500)
				return
			}
		}

		if masterPl != nil {
			w.Header().Set("Connection", "keep-alive")
			_, err = w.Write(masterPl.Encode().Bytes())
			return
		} else if mediaPl != nil {
			w.Header().Set("Connection", "keep-alive")
			_, err = w.Write(mediaPl.Encode().Bytes())
			return
		}
		if err != nil {
			glog.Errorf("Error writing playlist to ResponseWriter: %v", err)
			return
		}

		return
	}

	if strings.HasSuffix(r.URL.Path, ".ts") {
		seg, err := getSegment(r.URL)
		if err != nil {
			glog.Errorf("Error getting segment %v: %v", r.URL, err)
			return
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(r.URL.Path)))
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Connection", "keep-alive")
		_, err = w.Write(seg)
		if err != nil {
			glog.Errorf("Error writting HLS segment %v: %v", r.URL, err)
			return
		}
		return
	}

	http.Error(w, "Cannot find HTTP video resource: "+r.URL.String(), 500)
}

func handleVOD(url *url.URL, vodPath string, w http.ResponseWriter) error {
	if strings.HasSuffix(url.Path, ".m3u8") {
		plName := filepath.Join(vodPath, strings.Replace(url.Path, "/vod/", "", -1))
		dat, err := ioutil.ReadFile(plName)
		if err != nil {
			glog.Errorf("Cannot find file: %v", plName)
			return ErrNotFound
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(url.Path)))
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "max-age=5")
		w.Write(dat)
	}

	if strings.Contains(url.Path, ".ts") {
		segName := filepath.Join(vodPath, strings.Replace(url.Path, "/vod/", "", -1))
		dat, err := ioutil.ReadFile(segName)
		if err != nil {
			glog.Errorf("Cannot find file: %v", segName)
			return ErrNotFound
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(url.Path)))
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(dat)
	}

	return nil
}