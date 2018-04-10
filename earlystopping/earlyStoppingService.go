package earlystopping

import (
	"errors"
	"github.com/kubeflow/hp-tuning/api"
	vdb "github.com/kubeflow/hp-tuning/db"
	"log"
	"sort"
	"strconv"
)

type MedialStoppingParam struct {
	LeastStep int
	Margin    float64
}

type MedianStoppingRule struct {
	confList map[string]MedialStoppingParam
	dbIf     vdb.VizierDBInterface
}

func NewMedianStoppingRule() *MedianStoppingRule {
	m := &MedianStoppingRule{}
	m.confList = make(map[string]MedianStoppingRule)
	m.dbIf = vdb.New()
	return m
}

func (m *MedianStoppingRule) SetEarlyStoppingParameter(ctx context.Context, in *api.SetEarlyStoppingParameterRequest) (*api.SetEarlyStoppingParameterReply, error) {
	p := &MedialStoppingParam{}
	for _, ep := range in.EarlyStoppingParameters {
		switch ep.Name {
		case "LeastStep":
			p.LeastStep, _ = strconv.Atoi(ep.Value)
		case "Margin":
			p.Margin, _ = strconv.ParseFloat(ep.Value, 64)
		default:
			log.Printf("Unknown EarlyStopping Parameter %v", sp.Name)
		}
	}
	m.confList[in.StudyId] = p
	return &api.SetEarlyStoppingParameterReply{}, nil
}

func (m *MedianStoppingRule) getMedianRunningAverage(completedTrials []*api.Trial, step int) float64 {
	r := []float64{}
	for _, ct := range completedTrials {
		if ct.Status == api.TrialState_COMPLETED {
			var ra float64
			for s := 0; s < step; s++ {
				p, _ := strconv.ParseFloat(ct.EvalLogs[s].Value, 64)
				ra += p
			}
			ra = ra / float64(len(ct.EvalLogs))
			r = append(r, ra)
			var p float64
			if len(ct.EvalLogs) < step {
				p, _ = strconv.ParseFloat(ct.EvalLogs[len(ct.EvalLogs)-1].Value, 64)
			} else {
				p, _ = strconv.ParseFloat(ct.EvalLogs[step-1].Value, 64)
			}
			r = append(r, p)
		}
	}
	if len(r) == 0 {
		return 0
	} else {
		sort.Float64s(r)
		return r[len(r)/2]
	}
}

func (m *MedianStoppingRule) getBestValue(sc *api.StudyConfig, logs []*api.EvaluationLog) (float64, error) {
	if len(logs) == 0 {
		return 0, errors.New("Evaluation Log is missing")
	}
	ot := sc.OptimizationType
	if ot != api.OptimizationType_MAXIMIZE && ot != api.OptimizationType_MINIMIZE {
		return 0, errors.New("OptimizationType Unknown.")
	}
	var ret float64
	var isin bool = false
	for _, l := range logs {
		for _, m := range l.Metrics {
			if m.Name == sc.ObjectiveValueName {
				isin = true
				lv = strconv.ParseFloat(l.Value, 64)
				if ot == api.OptimizationType_MAXIMIZE && ret < ot {
					ret = ot
				} else if ot == api.OptimizationType_MINIMIZE && ret > ot {
					ret = ot
				}
			}
		}
	}
	if !isin {
		return 0, errors.New("No Objective value log inEvaluation Logs")
	}
	return ret
}
func (m *MedianStoppingRule) ShouldTrialStop(ctx context.Context, in *api.ShouldTrialStopRequest) (*api.ShouldTrialStopReply, error) {
	if _, ok := m.confList[in.StudyId]; !ok {
		return &api.ShouldTrialStopReply{}, errors.New("EarlyStopping config is not set.")
	}
	tl, err := m.dbIf.GetTrialList(in.StudyId)
	if err != nil {
		return &api.ShouldTrialStopReply{}, err
	}
	sc, err := m.dbIf.GetStudyConfig(in.StudyId)
	if err != nil {
		return &api.ShouldTrialStopReply{}, err
	}
	rtl := []*api.Trial{}
	ctl := []*api.Trial{}
	s_t := []*api.Trial{}
	for _, t := range tl {
		if t.Status == api.TrialState_RUNNING {
			rtl = append(rtl, t)
		}
		if t.Status == api.TrialState_COMPLETED {
			ctl = append(ctl, t)
		}
	}
	for _, t := range rtl {
		s := len(t.EvalLogs)
		if s < m.confList[in.StudyId].LeastStep {
			continue
		}
		om := m.getMedianRunningAverage(ctl, s)
		v, err := m.getBestValue(sc, t.EvalLogs)
		if err != nil {
			log.Printf("Fail to Get Best Value at %s: %v", t.TrialId, err)
			continue
		}
		fmt.Printf("Trial %v, Current value %v Median value in step %v %v\n", t.TrialId, v, s, om)
		if v < (om - m.confList[in.StudyId].Margin) {
			s_t = append(s_t, t)
		}
	}
	return &api.ShouldTrialStopReply{Trials: s_t}
}
