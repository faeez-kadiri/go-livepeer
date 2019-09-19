package verification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"

	"github.com/livepeer/go-livepeer/core"
	"github.com/livepeer/go-livepeer/net"

	"github.com/livepeer/lpms/ffmpeg"
	"github.com/livepeer/lpms/stream"
)

type VerificationResolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type VerificationRendition struct {
	Uri        string                 `json:"uri"`
	Resolution VerificationResolution `json:"resolution"`
	Framerate  uint                   `json:"frame_rate"`
	Pixels     int64                  `json:"pixels"`
}
type VerificationReq struct {
	Source         string                  `json:"source"`
	Renditions     []VerificationRendition `json:"renditions"`
	OrchestratorID string                  `json:"orchestratorID"`
	Model          string                  `json:"model"`
}

func Verify(mid core.ManifestID, source *stream.HLSSegment,
	profiles []ffmpeg.VideoProfile, res *net.TranscodeData) error {
	glog.Info("\n\n\nVerifying segment....\n")
	src := fmt.Sprintf("http://127.0.0.1:8935/stream/%s/source/%d.ts", mid, source.SeqNo)
	renditions := []VerificationRendition{}
	for i, v := range res.Segments {
		p := profiles[i]
		w, h, _ := ffmpeg.VideoProfileResolution(p) // XXX check err
		uri := fmt.Sprintf("http://127.0.0.1:8935/stream/%s/%s/%d.ts",
			mid, p.Name, source.SeqNo)
		r := VerificationRendition{
			Uri:        uri,
			Resolution: VerificationResolution{Width: w, Height: h},
			Framerate:  p.Framerate,
			Pixels:     v.Pixels,
		}
		renditions = append(renditions, r)
	}

	vreq := VerificationReq{
		Source:         src,
		Renditions:     renditions,
		OrchestratorID: "foo",
		Model:          "https://storage.googleapis.com/verification-models/verification.tar.xz",
	}
	vreqData, err := json.Marshal(vreq)
	if err != nil {
		glog.Error("Could not marshal JSON for verifier! ", err)
		return err
	}
	glog.Info("\nRequest Body\n", string(vreqData))
	resp, err := http.Post("http://localhost:5000/verify",
		"application/json", bytes.NewBuffer(vreqData))
	if err != nil {
		glog.Error("Could not submit response ", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	glog.Info("\n\nResponse Body\n", string(body))
	return nil
}
