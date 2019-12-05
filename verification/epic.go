package verification

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/glog"

	"github.com/livepeer/go-livepeer/common"

	"github.com/livepeer/lpms/ffmpeg"
)

var ErrVideoUnavailable = errors.New("VideoUnavailable")
var ErrAudioMismatch = Retryable{errors.New("AudioMismatch")}
var ErrTampered = Retryable{errors.New("Tampered")}

type epicResolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type epicRendition struct {
	URI        string         `json:"uri"`
	Resolution epicResolution `json:"resolution"`
	Framerate  uint           `json:"frame_rate"`
	Pixels     int64          `json:"pixels"`
}
type epicRequest struct {
	Source         string          `json:"source"`
	Renditions     []epicRendition `json:"renditions"`
	OrchestratorID string          `json:"orchestratorID"`
	Model          string          `json:"model"`
}

type epicResults struct {
	Source  string `json:"source"`
	Results []struct {
		VideoAvailable bool    `json:"video_available"`
		AudioAvailable bool    `json:"audio_available"`
		AudioDistance  float64 `json:"audio_dist"`
		Pixels         int64   `json:"pixels"`
		Tamper         float64 `json:"tamper"`
	} `json:"results"`
}

type verificationResult struct {
	score  float64
	pixels []int64
}

type EpicClassifier struct {
	Addr string
}

func (vr *verificationResult) Pixels() []int64 {
	return vr.pixels
}
func (vr *verificationResult) Score() float64 {
	return vr.score
}

func epicResultsToVerificationResults(er *epicResults) (*verificationResult, error) {
	// find average of scores and build list of pixels
	var (
		score  float64
		pixels []int64
	)
	var err error
	// If an error is gathered, continue to gather overall pixel counts
	// In case this is a false positive. Only return the first error.
	for _, v := range er.Results {
		// The order of error checking is somewhat arbitrary for now
		if v.VideoAvailable {
			if v.Tamper <= 0 {
				if err == nil {
					err = ErrTampered
				}
			}
		} else if v.AudioAvailable && v.AudioDistance != 0.0 {
			if err == nil {
				err = ErrAudioMismatch
			}
		} else {
			err = ErrVideoUnavailable
		}
		score += v.Tamper
		pixels = append(pixels, v.Pixels)
	}
	score = score / float64(len(er.Results))
	return &verificationResult{score: score, pixels: pixels}, err
}

func (e *EpicClassifier) Verify(params *VerifierParams) (VerificationResult, error) {
	mid, source, profiles := params.ManifestID, params.Source, params.Profiles
	orch, res := params.Orchestrator, params.Results
	glog.V(common.DEBUG).Infof("Verifying segment manifestID=%s seqNo=%d\n",
		mid, source.SeqNo)
	src := fmt.Sprintf("http://127.0.0.1:8935/stream/%s/source/%d.ts", mid, source.SeqNo)
	renditions := []epicRendition{}
	for i, v := range res.Segments {
		p := profiles[i]
		w, h, _ := ffmpeg.VideoProfileResolution(p) // XXX check err
		uri := fmt.Sprintf("http://127.0.0.1:8935/stream/%s/%s/%d.ts",
			mid, p.Name, source.SeqNo)
		r := epicRendition{
			URI:        uri,
			Resolution: epicResolution{Width: w, Height: h},
			Framerate:  p.Framerate,
			Pixels:     v.Pixels,
		}
		renditions = append(renditions, r)
	}

	oid := orch.Transcoder
	if orch.TicketParams != nil {
		oid = hex.EncodeToString(orch.TicketParams.Recipient)
	}
	req := epicRequest{
		Source:         src,
		Renditions:     renditions,
		OrchestratorID: oid,
		Model:          "https://storage.googleapis.com/verification-models/verification.tar.xz",
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		glog.Error("Could not marshal JSON for verifier! ", err)
		return nil, err
	}
	glog.V(common.DEBUG).Info("Request Body: ", string(reqData))
	startTime := time.Now()
	resp, err := http.Post(e.Addr, "application/json", bytes.NewBuffer(reqData))
	if err != nil {
		glog.Error("Could not submit request ", err)
		return nil, err
	}
	defer resp.Body.Close()
	var deferErr error // short variable re-declaration of `err` bites us with defer
	body, err := ioutil.ReadAll(resp.Body)
	endTime := time.Now()
	// `defer` param evaluation semantics force us into an anonymous function
	defer func() {
		glog.Infof("Verification complete manifestID=%s seqNo=%d err=%v dur=%v",
			mid, source.SeqNo, deferErr, endTime.Sub(startTime))
	}()
	if deferErr = err; err != nil {
		return nil, err
	}
	glog.V(common.DEBUG).Info("Response Body: ", string(body))
	var er epicResults
	err = json.Unmarshal(body, &er)
	if deferErr = err; err != nil {
		return nil, err
	}
	vr, err := epicResultsToVerificationResults(&er)
	deferErr = err
	return vr, err
}
