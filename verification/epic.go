package verification

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"

	"github.com/livepeer/lpms/ffmpeg"
)

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

type EpicClassifier struct {
	Addr string
}

func (e *EpicClassifier) Verify(params *VerifierParams) error {
	mid, source, profiles := params.ManifestID, params.Source, params.Profiles
	orch, res := params.Orchestrator, params.Results
	glog.Info("\n\n\nVerifying segment....\n")
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
		return err
	}
	glog.Info("\nRequest Body\n", string(reqData))
	resp, err := http.Post(e.Addr, "application/json", bytes.NewBuffer(reqData))
	if err != nil {
		glog.Error("Could not submit response ", err)
		return err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	glog.Info("\n\nResponse Body\n", string(body))
	return nil
}
