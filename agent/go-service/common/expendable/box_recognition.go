package expendable

import (
	"encoding/json"
	"fmt"
	"image"
	"strings"

	"github.com/MaaXYZ/MaaEnd/agent/go-service/pkg/recogtarget"
	maa "github.com/MaaXYZ/maa-framework-go/v4"
	"github.com/rs/zerolog/log"
)

const (
	boxComponentName   = "ExpendableBoxRecognition"
	attachVisitedBoxes = "visited_boxes"
)

var _ maa.CustomRecognitionRunner = &BoxRecognition{}

// BoxRecognition 是按坐标框去重的通用消费性识别器，每次返回首个尚未记录在 attach.visited_boxes 中的候选框。
type BoxRecognition struct{}

type boxParams struct {
	Candidate   string `json:"candidate"`
	VisitedNode string `json:"visited_node"`
}

// Run implements maa.CustomRecognitionRunner.
func (r *BoxRecognition) Run(ctx *maa.Context, arg *maa.CustomRecognitionArg) (*maa.CustomRecognitionResult, bool) {
	if ctx == nil || arg == nil || arg.Img == nil {
		log.Error().Str("component", boxComponentName).Msg("nil context, arg, or image")
		return nil, false
	}

	var p boxParams
	if err := json.Unmarshal([]byte(strings.TrimSpace(arg.CustomRecognitionParam)), &p); err != nil {
		log.Error().Err(err).Str("component", boxComponentName).Msg("parse params failed")
		return nil, false
	}
	p.Candidate = strings.TrimSpace(p.Candidate)
	if p.Candidate == "" {
		log.Error().Err(fmt.Errorf("candidate is required")).Str("component", boxComponentName).Msg("invalid params")
		return nil, false
	}
	visitedOwner := strings.TrimSpace(p.VisitedNode)
	if visitedOwner == "" {
		visitedOwner = strings.TrimSpace(arg.CurrentTaskName)
	}
	if visitedOwner == "" {
		log.Error().Str("component", boxComponentName).Msg("visited target node is empty")
		return nil, false
	}

	rawOwner, err := ctx.GetNodeJSON(visitedOwner)
	if err != nil {
		log.Error().Err(err).Str("component", boxComponentName).Str("node", visitedOwner).Msg("get visited node json failed")
		return nil, false
	}
	var nodeData struct {
		Attach map[string]json.RawMessage `json:"attach"`
	}
	if err := json.Unmarshal([]byte(rawOwner), &nodeData); err != nil {
		log.Error().Err(err).Str("component", boxComponentName).Str("node", visitedOwner).Msg("unmarshal visited node json failed")
		return nil, false
	}
	var visited []maa.Rect
	if rawVisited, ok := nodeData.Attach[attachVisitedBoxes]; ok && len(rawVisited) > 0 {
		if err := json.Unmarshal(rawVisited, &visited); err != nil {
			log.Error().Err(err).Str("component", boxComponentName).Str("node", visitedOwner).Msg("unmarshal visited_boxes failed")
			return nil, false
		}
	}

	detail, err := ctx.RunRecognition(p.Candidate, arg.Img)
	if err != nil {
		log.Error().Err(err).Str("component", boxComponentName).Str("candidate", p.Candidate).Msg("RunRecognition failed")
		return nil, false
	}
	if detail == nil || !detail.Hit {
		return nil, false
	}
	selected, err := recogtarget.SelectDetail(ctx, p.Candidate, detail)
	if err != nil {
		log.Error().Err(err).Str("component", boxComponentName).Str("candidate", p.Candidate).Msg("select candidate detail failed")
		return nil, false
	}

	box, ok := nextUnvisitedBox(selected, visited)
	if !ok {
		return nil, false
	}

	attachPatch := make(map[string]any, len(nodeData.Attach)+1)
	for k, v := range nodeData.Attach {
		attachPatch[k] = v
	}
	attachPatch[attachVisitedBoxes] = append(visited, box)
	if err := ctx.OverridePipeline(map[string]any{
		visitedOwner: map[string]any{"attach": attachPatch},
	}); err != nil {
		log.Error().Err(err).Str("component", boxComponentName).Str("node", visitedOwner).Msg("save visited_boxes failed")
		return nil, false
	}

	return &maa.CustomRecognitionResult{Box: box}, true
}

func nextUnvisitedBox(detail *maa.RecognitionDetail, visited []maa.Rect) (maa.Rect, bool) {
	if detail == nil || !detail.Hit {
		return maa.Rect{}, false
	}
	if detail.Results != nil {
		for _, res := range detail.Results.Filtered {
			if box, ok := resultBox(res); ok && box[2] > 0 && box[3] > 0 && !boxVisited(box, visited) {
				return box, true
			}
		}
	}
	if detail.Box[2] > 0 && detail.Box[3] > 0 && !boxVisited(detail.Box, visited) {
		return detail.Box, true
	}
	return maa.Rect{}, false
}

func resultBox(res *maa.RecognitionResult) (maa.Rect, bool) {
	if res == nil {
		return maa.Rect{}, false
	}
	if v, ok := res.AsOCR(); ok && v != nil {
		return v.Box, true
	}
	if v, ok := res.AsTemplateMatch(); ok && v != nil {
		return v.Box, true
	}
	if v, ok := res.AsNeuralNetworkDetect(); ok && v != nil {
		return v.Box, true
	}
	if v, ok := res.AsCustom(); ok && v != nil {
		return v.Box, true
	}
	return maa.Rect{}, false
}

func boxVisited(box maa.Rect, visited []maa.Rect) bool {
	pt := image.Pt(box[0]+box[2]/2, box[1]+box[3]/2)
	cur := image.Rect(box[0], box[1], box[0]+box[2], box[1]+box[3])
	for _, old := range visited {
		if old[2] <= 0 || old[3] <= 0 {
			continue
		}
		if pt.In(image.Rect(old[0], old[1], old[0]+old[2], old[1]+old[3])) ||
			image.Pt(old[0]+old[2]/2, old[1]+old[3]/2).In(cur) {
			return true
		}
	}
	return false
}
