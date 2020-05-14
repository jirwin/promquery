package promquery

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
	promql.Inspect(q.expr, func(n promql.Node, _ []promql.Node) error {
		if n == nil {
			return nil
		}

		switch n.(type) {
		case *promql.VectorSelector:
			vs, ok := n.(*promql.VectorSelector)
			if !ok {
				return nil
			}
			vs.LabelMatchers = append(vs.LabelMatchers, matcher)
		case *promql.MatrixSelector:
			ms, ok := n.(*promql.MatrixSelector)
			if !ok {
				return nil
			}
			ms.LabelMatchers = append(ms.LabelMatchers, matcher)
		}

		return nil
	})
}

func (q *PromQuery) String() string {
	return q.expr.String()
}

func NewPromQuery(query string) (*PromQuery, error) {
	expr, err := promql.ParseExpr(query)
	if err != nil {
		return nil, err
	}

	return &PromQuery{
		query: query,
		expr:  expr,
	}, nil
}
