package query

import (
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
)

// Proposed metric format in yaml
// Monitoring:
//   - sum(ztap_active_configs) - sum(ztap_total_configs)

type PromQuery struct {
	query string
	expr  promql.Expr
}

func (q *PromQuery) getMetrics() []*promql.VectorSelector {
	metrics := []*promql.VectorSelector{}
	promql.Inspect(q.expr, func(n promql.Node) bool {
		if n == nil {
			return false
		}

		switch expr := n.(type) {
		case *promql.VectorSelector:
			metrics = append(metrics, expr)
			return false

		default:
			return true
		}
	})

	return metrics
}

func (q *PromQuery) AddLabel(name, value string, equal bool) error {
	mt := labels.MatchEqual
	if !equal {
		mt = labels.MatchNotEqual
	}

	matcher, err := labels.NewMatcher(mt, name, value)
	if err != nil {
		return err
	}

	q.appendLabel(matcher)

	return nil
}

func (q *PromQuery) AddRegexpLabel(name, value string, equal bool) error {
	mt := labels.MatchRegexp
	if !equal {
		mt = labels.MatchNotRegexp
	}

	matcher, err := labels.NewMatcher(mt, name, value)
	if err != nil {
		return err
	}

	q.appendLabel(matcher)

	return nil
}

func (q *PromQuery) appendLabel(matcher *labels.Matcher) {
	metrics := q.getMetrics()
	for _, metric := range metrics {
		metric.LabelMatchers = append(metric.LabelMatchers, matcher)
	}
}

func (q *PromQuery) String() string {
	return q.expr.String()
}

func New(query string) (*PromQuery, error) {
	expr, err := promql.ParseExpr(query)
	if err != nil {
		return nil, err
	}

	return &PromQuery{
		query: query,
		expr:  expr,
	}, nil
}
