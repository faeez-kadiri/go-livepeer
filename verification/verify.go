package verification

import (
	"errors"
	"sort"

	"github.com/livepeer/go-livepeer/core"
	"github.com/livepeer/go-livepeer/net"

	"github.com/livepeer/lpms/ffmpeg"
	"github.com/livepeer/lpms/stream"
)

// Special error type indicating a retryable error
// Such errors typically mean re-trying the transcode might help
// (Non-retryable errors usually indicate unrecoverable system errors)
type Retryable struct {
	error
}

var ErrPixelMismatch = Retryable{errors.New("PixelMismatch")}

type VerifierParams struct {
	// ManifestID should go away once we do direct push of video
	ManifestID core.ManifestID

	// Bytes of the source video segment
	Source *stream.HLSSegment

	// Rendition parameters to be checked
	Profiles []ffmpeg.VideoProfile

	// Information on the orchestrator that performed the transcoding
	Orchestrator *net.OrchestratorInfo

	// Transcoded result metadata
	Results *net.TranscodeData
}

type VerificationResult interface {
	Score() float64

	// Number of pixels decoded in this result
	Pixels() []int64
}

type Verifier interface {
	Verify(params *VerifierParams) (VerificationResult, error)
}

type Policy struct {

	// Verification function to run
	Verifier Verifier

	// Maximum number of retries until the policy chooses a winner
	Retries int

	// How often to invoke the verifier, on a per-segment basis
	SampleRate float64 // XXX for later

	// How many parallel transcodes to support
	Redundancy int // XXX for later
}

type SegmentVerifierResults struct {
	params *VerifierParams
	res    VerificationResult
}

type byResScore []SegmentVerifierResults

func (a byResScore) Len() int           { return len(a) }
func (a byResScore) Swap(i, j int)      { a[i], a[j] = a[j], a[j] }
func (a byResScore) Less(i, j int) bool { return a[i].res.Score() < a[j].res.Score() }

type SegmentVerifier struct {
	policy  *Policy
	results []SegmentVerifierResults
	count   int
}

func NewSegmentVerifier(p *Policy) *SegmentVerifier {
	return &SegmentVerifier{policy: p}
}

func (sv *SegmentVerifier) Verify(params *VerifierParams) (*VerifierParams, error) {

	// TODO sig checking; extract from broadcast.go

	// TODO Use policy sampling rate to determine whether to invoke verifier
	//      If not, exit early
	res, err := sv.policy.Verifier.Verify(params)

	// Check pixel counts
	if res != nil && err == nil {
		if len(res.Pixels()) != len(params.Results.Segments) {
			// TODO make allowances for the verification algos not doing
			//      pixel counts themselves; adapt broadcast.go verifyPixels
			return params, nil
		}
		for i, v := range res.Pixels() {
			if v != params.Results.Segments[i].Pixels {
				err = ErrPixelMismatch
			}
		}
	}

	if err == nil {
		// Verification passed successfully, so use this set of params
		return params, nil
	}
	sv.count++

	// Append retryable errors to results
	// The caller should terminate processing for non-retryable errors
	if _, retry := err.(Retryable); retry {
		r := SegmentVerifierResults{params: params, res: res}
		sv.results = append(sv.results, r)
	}

	// Check for max retries
	// If max hit, return best params so far
	if sv.count >= sv.policy.Retries {
		if len(sv.results) <= 0 {
			return nil, nil
		}
		sort.Sort(byResScore(sv.results))
		return sv.results[0].params, err
	}

	return nil, err
}
