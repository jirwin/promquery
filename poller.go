package promquery

import (
	"context"
	"math"
	"time"

	"sync"

	"fmt"

	"strconv"

	"math/rand"

	"github.com/ScaleFT/monotime"
	"github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

const defaultLookback = -30 * time.Second

type Poller struct {
	client          prometheus.API
	Queries         []*PromQuery
	SuccessCount    int
	initialValueMap sync.Map
	initialFoundMap sync.Map
	stddevMap       sync.Map
	timer           monotime.Timer
}

func (p *Poller) query(ctx context.Context, q string) (<-chan string, <-chan error) {
	resultChan := make(chan string)
	errChan := make(chan error)
	go func() {
		// Look at metrics 30 seconds in the past so that we get values that have been completely scraped
		// and won't be changing as often as the most recent metrics. This also prevents us from getting 0
		// metrics from calls to Query when we should get consistent metric returns
		t := time.Now().Add(defaultLookback)
		val, _, err := p.client.Query(ctx, q, t)
		if err != nil {
			errChan <- err
			return
		}

		if v, ok := val.(model.Vector); ok {
			if len(v) != 1 {
				errChan <- fmt.Errorf("queries must return exactly one metric")
				return
			}

			resultChan <- v[0].Value.String()
		} else {
			errChan <- fmt.Errorf("got unexpected result from %s: %#v", q, v)
		}
	}()

	return resultChan, errChan
}

func (p *Poller) query_range(ctx context.Context, q string, start, end time.Time, step time.Duration) (<-chan []string, <-chan error) {
	resultChan := make(chan []string)
	errChan := make(chan error)
	go func() {
		// Look at metrics 30 seconds in the past so that we get values that have been completely scraped
		// and won't be changed as often as most recent metrics
		qrange := prometheus.Range{
			Start: start.Add(defaultLookback),
			End:   end.Add(defaultLookback),
			Step:  step,
		}
		val, _, err := p.client.QueryRange(ctx, q, qrange)
		if err != nil {
			errChan <- err
			return
		}

		if v, ok := val.(model.Matrix); ok {
			if len(v) == 0 {
				resultChan <- []string{}
				return
			}

			if len(v) != 1 {
				errChan <- fmt.Errorf("queries must return exactly one metric")
				return
			}

			values := []string{}
			for _, metricVal := range v[0].Values {
				values = append(values, metricVal.Value.String())
			}
			resultChan <- values
		} else {
			errChan <- fmt.Errorf("got unexpected result from %s: %#v", q, v)
		}
	}()

	return resultChan, errChan
}

func (p *Poller) calcStdDev(vals []string) (float64, float64, error) {
	floatVals := make([]float64, len(vals))

	for i, v := range vals {
		floatVal, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, 0, err
		}

		floatVals[i] = floatVal
	}

	var sum, squaredSum, count float64
	for _, v := range floatVals {
		sum += v
		squaredSum += v * v
		count++
	}
	avg := sum / count

	// If we're dealing with smaller numbers we can hit the floating point precision limit
	// or a negative number in the square root, in which case return 0
	stdDev := math.Sqrt(float64(squaredSum/count - avg*avg))
	if math.IsNaN(stdDev) {
		stdDev = float64(0.0)
	}

	return stdDev, floatVals[len(floatVals)-1], nil
}

func (p *Poller) getInitValues(ctx context.Context, q *PromQuery) error {
	now := time.Now().UTC()
	resultChan, errChan := p.query_range(ctx, q.String(), now.Add(-30*time.Minute), now, 30*time.Second)

	select {
	case <-ctx.Done():
		return fmt.Errorf("context was cancelled")

	case result := <-resultChan:
		if len(result) == 0 {
			fmt.Printf("No initial values for %s found, will skip while waiting\n", q.String())
			p.initialFoundMap.Store(q.String(), false)
			return nil
		}

		stddev, latest, err := p.calcStdDev(result)
		if err != nil {
			return err
		}

		p.initialFoundMap.Store(q.String(), true)
		p.stddevMap.Store(q.String(), stddev)
		p.initialValueMap.Store(q.String(), latest)

		fmt.Printf("Initial values for %s: stddev: %0.4f, initial val: %0.4f\n", q.String(), stddev, latest)

	case err := <-errChan:
		return err
	}

	return nil
}

func (p *Poller) Init(ctx context.Context) {
	fmt.Println("Gathering initial values...")

	wg := sync.WaitGroup{}
	for _, q := range p.Queries {
		wg.Add(1)
		go func(q *PromQuery) {
			defer wg.Done()
			err := p.getInitValues(ctx, q)
			if err != nil {
				fmt.Printf("error getting stddev for %s: %s\n", q.String(), err.Error())
				return
			}
		}(q)
	}

	wg.Wait()
}

func (p *Poller) Wait(ctx context.Context, interval time.Duration, notifyMetrics func(string)) <-chan error {
	doneChan := make(chan error)
	go func() {
		p.timer = monotime.New()

		wait := make(chan struct{})
		waitErr := make(chan error)
		ctx1, canc := context.WithCancel(ctx)
		defer canc()

		go func() {
			wg := sync.WaitGroup{}

			// If there aren't any queries, wait a single polling period before continuing.
			if len(p.Queries) == 0 {
				<-time.After(interval)
			} else {
				for _, q := range p.Queries {
					foundMapItem, ok := p.initialFoundMap.Load(q.String())
					if !ok {
						waitErr <- fmt.Errorf("error: unable to find found entry for query(%s)", q.String())
						continue
					}
					found, ok := foundMapItem.(bool)
					if !ok {
						waitErr <- fmt.Errorf("error: found entry for query(%s) is invalid", q.String())
						continue
					}

					if !found {
						fmt.Printf("didn't find initial values for query(%s), not waiting for it to resolve\n", q.String())
						continue
					}

					wg.Add(1)
					go func(q *PromQuery) {
						defer wg.Done()
						stddevMapItem, ok := p.stddevMap.Load(q.String())
						if !ok {
							waitErr <- fmt.Errorf("error: unable to find stddev for query(%s)", q.String())
							return
						}
						stddev, ok := stddevMapItem.(float64)
						if !ok {
							waitErr <- fmt.Errorf("error: stddev for query(%s) is invalid", q.String())
							return
						}

						initValMapItem, ok := p.initialValueMap.Load(q.String())
						if !ok {
							waitErr <- fmt.Errorf("error: unable to find initial value for query(%s)", q.String())
							return
						}
						initVal, ok := initValMapItem.(float64)
						if !ok {
							waitErr <- fmt.Errorf("error: initial value for query(%s) is invalid", q.String())
							return
						}

						// Used to track how many queries have been returned within the standard deviation
						successful := 0

						for {
							fmt.Printf("Waiting %s (%s elapsed) to poll for metrics\n", interval, p.timer.Elapsed())
							<-time.After(interval)

							resultChan, errChan := p.query(ctx1, q.String())
							select {
							case <-ctx1.Done():
								waitErr <- fmt.Errorf("error while polling metrics: %s", ctx1.Err().Error())
								return

							case result := <-resultChan:
								resVal, err := strconv.ParseFloat(result, 64)
								if err != nil {
									waitErr <- err
									successful = 0
									continue
								}

								if notifyMetrics != nil {
									fmt.Printf("initial value %g, current value %g (%s elapsed)", initVal, resVal, p.timer.Elapsed())
									notifyMetrics(fmt.Sprintf("initial value %g, current value %g (%s elapsed)", initVal, resVal, p.timer.Elapsed()))
								}

								delta := math.Abs(initVal - resVal)
								if delta <= stddev {
									if successful >= p.SuccessCount {
										return
									}
									successful++
									continue
								} else {
									successful = 0
									continue
								}

							case err := <-errChan:
								fmt.Printf("error while polling %s: %s\n", q.String(), err.Error())
								successful = 0
								continue
							}
						}
					}(q)
				}
			}

			wg.Wait()
			close(wait)
		}()

		select {
		case <-wait:
			doneChan <- nil

		case err := <-waitErr:
			doneChan <- err

		case <-ctx.Done():
			doneChan <- fmt.Errorf("context was cancelled before polling was complete: %s", ctx.Err().Error())
		}
	}()

	return doneChan
}

func NewPoller(addrs []string, queries []string, successCount int) (*Poller, error) {
	// We are given a slice of possible addrs to connect to. Pick a random one and go with it.
	addr := addrs[rand.Intn(len(addrs))]
	client, err := api.NewClient(api.Config{
		Address: addr,
	})
	if err != nil {
		return nil, err
	}

	// de-duplicate queries so we don't do extra work
	seen := make(map[string]bool)
	dedupedQueries := []string{}
	for _, q := range queries {
		if ok := seen[q]; ok {
			continue
		}
		dedupedQueries = append(dedupedQueries, q)
		seen[q] = true
	}

	parsedQueries := make([]*PromQuery, len(dedupedQueries))
	for i, q := range dedupedQueries {
		pq, err := NewPromQuery(q)
		if err != nil {
			return nil, err
		}

		parsedQueries[i] = pq
	}

	return &Poller{
		client:       prometheus.NewAPI(client),
		Queries:      parsedQueries,
		SuccessCount: successCount,
	}, nil
}
