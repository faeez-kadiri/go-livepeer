package core

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/livepeer/go-livepeer/common"
	"github.com/livepeer/go-livepeer/monitor"
	"github.com/livepeer/lpms/ffmpeg"

	"github.com/golang/glog"
)

type Transcoder interface {
	Transcode(job string, fname string, profiles []ffmpeg.VideoProfile) (*TranscodeData, error)
}

type LocalTranscoder struct {
	workDir string
}

func (lt *LocalTranscoder) Transcode(job string, fname string, profiles []ffmpeg.VideoProfile) (*TranscodeData, error) {
	// Set up in / out config
	in := &ffmpeg.TranscodeOptionsIn{
		Fname: fname,
		Accel: ffmpeg.Software,
	}
	opts := profilesToTranscodeOptions(lt.workDir, ffmpeg.Software, profiles)

	_, seqNo, parseErr := parseURI(fname)
	start := time.Now()

	res, err := ffmpeg.Transcode3(in, opts)
	if err != nil {
		return nil, err
	}

	if monitor.Enabled && parseErr == nil {
		// This will run only when fname is actual URL and contains seqNo in it.
		// When orchestrator works as transcoder, `fname` will be relative path to file in local
		// filesystem and will not contain seqNo in it. For that case `SegmentTranscoded` will
		// be called in orchestrator.go
		monitor.SegmentTranscoded(0, seqNo, time.Since(start), common.ProfilesNames(profiles))
	}

	return resToTranscodeData(res, opts)
}

func NewLocalTranscoder(workDir string) Transcoder {
	return &LocalTranscoder{workDir: workDir}
}

type lb interface {
	choose(sess string, cost int) (int, int, error)
	complete(i int, cost int)
	terminate(sess string, gpu int)
}

type nvidiaSession struct {
	used       time.Time
	transcoder *ffmpeg.Transcoder
	key        string
	nonce      string
}

type NvidiaTranscoder struct {
	workDir string
	devices []string
	lb      lb

	// The following fields need to be protected by the mutex `mu`
	mu       *sync.RWMutex
	sessions map[string]*nvidiaSession
}

func (nv *NvidiaTranscoder) Transcode(job string, fname string, profiles []ffmpeg.VideoProfile) (*TranscodeData, error) {
	// Estimate cost up front for the LB algo.
	costEstimate := 0
	for _, v := range profiles {
		// This resolution parsing is expensive; would be best to do it just once!
		w, h, err := ffmpeg.VideoProfileResolution(v)
		if err != nil {
			continue
		}
		costEstimate += w * h * int(v.Framerate) // TODO incorporate duration
	}
	gpu, computedCost, err := nv.lb.choose(job, costEstimate)
	if err != nil {
		return nil, err
	}
	defer nv.lb.complete(gpu, computedCost)

	// Local cleanup function
	cleanupSession := func(session *nvidiaSession) {
		nv.mu.RLock()
		sess, exists := nv.sessions[session.key]
		nv.mu.RUnlock()
		if !exists {
			return
		}
		if sess.nonce != session.nonce {
			// Session cached at `key` has since been recreated
			return
		}
		// Ordering is very important here!
		// StopTranscoder may take a long time to execute
		nv.mu.Lock()
		delete(nv.sessions, session.key)
		nv.mu.Unlock()
		nv.lb.terminate(job, gpu)
		session.transcoder.StopTranscoder()
		glog.V(common.DEBUG).Info("LB: Deleted transcode session for ", session.key)
	}

	// Acquire transcode session. Map to job id + assigned GPU
	key := job + "_" + nv.devices[gpu]
	nonce := common.RandName() // because key alone is prone to reuse
	nv.mu.Lock()
	session, exists := nv.sessions[key]
	if exists {
		// Transcode session exists, so just reset last used time
		glog.V(common.DEBUG).Info("LB: Using existing transcode session for ", key)
		session.used = time.Now()
	} else {
		// No transcode session exists, so create one
		glog.V(common.DEBUG).Info("LB: Creating transcode session for ", key)
		session = &nvidiaSession{
			used:       time.Now(),
			transcoder: ffmpeg.NewTranscoder(),
			key:        key,
			nonce:      nonce,
		}
		nv.sessions[key] = session

		// Launch cleanup monitoring routine
		go func() {
			// Terminate the session after a period of inactivity
			interval := 1 * time.Minute
			ticker := time.NewTicker(interval)
			for range ticker.C {
				nv.mu.RLock()
				used := session.used
				nv.mu.RUnlock()
				if time.Since(used) > interval {
					break
				}
			}
			glog.V(common.DEBUG).Info("LB: Stopping transcoder due to timeout for ", key)
			cleanupSession(session)
		}()
	}
	nv.mu.Unlock()

	// Set up in / out config
	in := &ffmpeg.TranscodeOptionsIn{
		Fname:  fname,
		Accel:  ffmpeg.Nvidia,
		Device: nv.devices[gpu],
	}
	opts := profilesToTranscodeOptions(nv.workDir, ffmpeg.Nvidia, profiles)

	// Do the Transcoding
	res, err := session.transcoder.Transcode(in, opts)
	if err != nil {
		glog.V(common.DEBUG).Info("LB: Stopping transcoder due to error for ", key)
		cleanupSession(session)
		return nil, err
	}

	return resToTranscodeData(res, opts)
}

func NewNvidiaTranscoder(devices string, workDir string) Transcoder {
	d := strings.Split(devices, ",")
	return &NvidiaTranscoder{
		workDir: workDir,
		devices: d,
		lb:       NewRRLoadBalancer(len(d)),
		mu:       &sync.RWMutex{},
		sessions: make(map[string]*nvidiaSession),
	}
}

func parseURI(uri string) (string, uint64, error) {
	var mid string
	var seqNo uint64
	parts := strings.Split(uri, "/")
	if len(parts) < 3 {
		return mid, seqNo, fmt.Errorf("BadURI")
	}
	mid = parts[len(parts)-2]
	parts = strings.Split(parts[len(parts)-1], ".")
	seqNo, err := strconv.ParseUint(parts[0], 10, 64)
	return mid, seqNo, err
}

func resToTranscodeData(res *ffmpeg.TranscodeResults, opts []ffmpeg.TranscodeOptions) (*TranscodeData, error) {
	if len(res.Encoded) != len(opts) {
		return nil, errors.New("lengths of results and options different")
	}

	// Convert results into in-memory bytes following the expected API
	segments := make([]*TranscodedSegmentData, len(opts), len(opts))
	for i := range opts {
		oname := opts[i].Oname
		o, err := ioutil.ReadFile(oname)
		if err != nil {
			glog.Error("Cannot read transcoded output for ", oname)
			return nil, err
		}
		segments[i] = &TranscodedSegmentData{Data: o, Pixels: res.Encoded[i].Pixels}
		os.Remove(oname)
	}

	return &TranscodeData{
		Segments: segments,
		Pixels:   res.Decoded.Pixels,
	}, nil
}

func profilesToTranscodeOptions(workDir string, accel ffmpeg.Acceleration, profiles []ffmpeg.VideoProfile) []ffmpeg.TranscodeOptions {
	opts := make([]ffmpeg.TranscodeOptions, len(profiles), len(profiles))
	for i := range profiles {
		o := ffmpeg.TranscodeOptions{
			Oname:        fmt.Sprintf("%s/out_%s.ts", workDir, common.RandName()),
			Profile:      profiles[i],
			Accel:        accel,
			AudioEncoder: ffmpeg.ComponentOptions{Name: "copy"},
		}
		opts[i] = o
	}
	return opts
}
