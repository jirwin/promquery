package promquery

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

func TestGetMetrics(t *testing.T) {
	q, err := NewPromQuery(`sum(foo_metric{role="bar"}) - sum(qux_metric{role!="baz"} - up) - 3`)
	require.NoError(t, err)

	metrics := q.getMetrics()
	require.Equal(t, 3, len(metrics))

	require.Equal(t, "foo_metric", metrics[0].Name)
	require.Equal(t, "role", metrics[0].LabelMatchers[0].Name)
	require.Equal(t, labels.MatchEqual, metrics[0].LabelMatchers[0].Type)
	require.Equal(t, "bar", metrics[0].LabelMatchers[0].Value)

	require.Equal(t, "qux_metric", metrics[1].Name)
	require.Equal(t, "role", metrics[1].LabelMatchers[0].Name)
	require.Equal(t, labels.MatchNotEqual, metrics[1].LabelMatchers[0].Type)
	require.Equal(t, "baz", metrics[1].LabelMatchers[0].Value)
}

func TestAddRegexpLabel(t *testing.T) {
	q, err := NewPromQuery(`foo_metric{role="bar"} - qux_metric{role!="baz"}`)
	require.NoError(t, err)

	metrics := q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 2, len(metrics[0].LabelMatchers))
	require.Equal(t, 2, len(metrics[1].LabelMatchers))

	q.AddRegexpLabel("az", ".*us-east-1.*", true)

	metrics = q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 3, len(metrics[0].LabelMatchers))
	require.Equal(t, 3, len(metrics[1].LabelMatchers))

	require.Equal(t, `foo_metric{az=~".*us-east-1.*",role="bar"} - qux_metric{az=~".*us-east-1.*",role!="baz"}`, q.String())
}

func TestAddNonEqualRegexpLabel(t *testing.T) {
	q, err := NewPromQuery(`foo_metric{role="bar"} - qux_metric{role!="baz"}`)
	require.NoError(t, err)

	metrics := q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 2, len(metrics[0].LabelMatchers))
	require.Equal(t, 2, len(metrics[1].LabelMatchers))

	q.AddRegexpLabel("az", ".*us-east-1.*", false)

	metrics = q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 3, len(metrics[0].LabelMatchers))
	require.Equal(t, 3, len(metrics[1].LabelMatchers))

	require.Equal(t, `foo_metric{az!~".*us-east-1.*",role="bar"} - qux_metric{az!~".*us-east-1.*",role!="baz"}`, q.String())
}

func TestAddEqualLabel(t *testing.T) {
	q, err := NewPromQuery(`foo_metric{role="bar"} - qux_metric{role!="baz"}`)
	require.NoError(t, err)

	metrics := q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 2, len(metrics[0].LabelMatchers))
	require.Equal(t, 2, len(metrics[1].LabelMatchers))

	q.AddLabel("az", "us-east-1", true)

	metrics = q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 3, len(metrics[0].LabelMatchers))
	require.Equal(t, 3, len(metrics[1].LabelMatchers))

	require.Equal(t, `foo_metric{az="us-east-1",role="bar"} - qux_metric{az="us-east-1",role!="baz"}`, q.String())
}

func TestAddNonEqualLabel(t *testing.T) {
	q, err := NewPromQuery(`foo_metric{role="bar"} - qux_metric{role!="baz"}`)
	require.NoError(t, err)

	metrics := q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 2, len(metrics[0].LabelMatchers))
	require.Equal(t, 2, len(metrics[1].LabelMatchers))

	q.AddLabel("az", "us-east-1", false)

	metrics = q.getMetrics()
	require.Equal(t, 2, len(metrics))
	require.Equal(t, 3, len(metrics[0].LabelMatchers))
	require.Equal(t, 3, len(metrics[1].LabelMatchers))

	require.Equal(t, `foo_metric{az!="us-east-1",role="bar"} - qux_metric{az!="us-east-1",role!="baz"}`, q.String())
}
