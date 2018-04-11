package earlystopping

import (
	"context"
	"errors"
	"github.com/kubeflow/hp-tuning/api"
	vdb "github.com/kubeflow/hp-tuning/db"
	"log"
	"sort"
	"strconv"
)

type MedianStoppingParam struct {
	LeastStep int
	Margin    float64
}

type MedianStoppingRule struct {
	confList map[string]*MedianStoppingParam
	dbIf     vdb.VizierDBInterface
}

func NewMedianStoppingRule() *MedianStoppingRule {
	m := &MedianStoppingRule{}
	m.confList = make(map[string]*MedianStoppingParam)
	m.dbIf = vdb.New()
	return m
}

func (m *MedianStoppingRule) SetEarlyStoppingParameter(ctx context.Context, in *api.SetEarlyStoppingParameterRequest) (*api.SetEarlyStoppingParameterReply, error) {
	p := &MedianStoppingParam{}
	for _, ep := range in.EarlyStoppingParameters {
		switch ep.Name {
		case "LeastStep":
			p.LeastStep, _ = strconv.Atoi(ep.Value)
		case "Margin":
			p.Margin, _ = strconv.ParseFloat(ep.Value, 64)
		default:
			log.Printf("Unknown EarlyStopping Parameter %v", ep.Name)
		}
	}
	m.confList[in.StudyId] = p
	return &api.SetEarlyStoppingParameterReply{}, nil
}

func (m *MedianStoppingRule) getMedianRunningAverage(sc *api.StudyConfig, completedTrialslogs [][]float64, step int) float64 {
	r := []float64{}
	for _, ctl := range completedTrialslogs {
		var ra float64
		var st int
		if step > len(ctl) {
			st = len(ctl)
		} else {
			st = step
		}
		for s := 0; s < st; s++ {
			ra += ctl[s]
		}
		ra = ra / float64(st)
		r = append(r, ra)
	}
	if len(r) == 0 {
		return 0
	} else {
		sort.Float64s(r)
		return r[len(r)/2]
	}
}

func (m *MedianStoppingRule) getBestValue(sc *api.StudyConfig, logs []*api.EvaluationLog) (float64, int, error) {
	if len(logs) == 0 {
		return 0, 0, errors.New("Evaluation Log is missing")
	}
	ot := sc.OptimizationType
	if ot != api.OptimizationType_MAXIMIZE && ot != api.OptimizationType_MINIMIZE {
		return 0, 0, errors.New("OptimizationType Unknown.")
	}
	var ret float64
	var target_objlog []float64
	for _, l := range logs {
		for _, m := range l.Metrics {
			if m.Name == sc.ObjectiveValueName {
				v, _ := strconv.ParseFloat(m.Value, 64)
				target_objlog = append(target_objlog, v)
			}
		}
	}
	if len(target_objlog) == 0 {
		return 0, 0, errors.New("No Objective value log inEvaluation Logs")
	}
	sort.Float64s(target_objlog)
	if ot == api.OptimizationType_MAXIMIZE {
		ret = target_objlog[len(target_objlog)-1]
	} else if ot == api.OptimizationType_MINIMIZE {
		ret = target_objlog[0]
	}
	return ret, len(target_objlog), nil
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
	ctl := make([][]float64, 0, len(tl))
	s_t := []*api.Trial{}
	for _, t := range tl {
		if t.Status == api.TrialState_RUNNING {
			rtl = append(rtl, t)
		}
		if t.Status == api.TrialState_COMPLETED {
			ctl = append(ctl, make([]float64, 0, len(t.EvalLogs)))
			for _, e := range t.EvalLogs {
				for _, m := range e.Metrics {
					if m.Name == sc.ObjectiveValueName {
						v, _ := strconv.ParseFloat(m.Value, 64)
						ctl[len(ctl)-1] = append(ctl[len(ctl)-1], v)
					}
				}
			}
		}
	}
	for _, t := range rtl {
		v, s, err := m.getBestValue(sc, t.EvalLogs)
		if err != nil {
			log.Printf("Fail to Get Best Value at %s: %v", t.TrialId, err)
			continue
		}
		if s < m.confList[in.StudyId].LeastStep {
			continue
		}
		om := m.getMedianRunningAverage(sc, ctl, s)
		log.Printf("Trial %v, Current value %v Median value in step %v %v\n", t.TrialId, v, s, om)
		if v < (om - m.confList[in.StudyId].Margin) {
			s_t = append(s_t, t)
		}
	}
	return &api.ShouldTrialStopReply{Trials: s_t}, nil
}
