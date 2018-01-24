package promquery

import (
	"context"
	"time"

	"sync"

	"fmt"

	"github.com/ScaleFT/monotime"
	"github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

type Poller struct {
	client        prometheus.API
	queries       []*PromQuery
	initialValues sync.Map
	timer         monotime.Timer
}

func (p *Poller) query(ctx context.Context, q string) (<-chan string, <-chan error) {
	resultChan := make(chan string)
	errChan := make(chan error)
	go func() {
		val, err := p.client.Query(ctx, q, time.Now())
		if err != nil {
			errChan <- err
			return
		}

		if v, ok := val.(model.Vector); ok {
			if len(v) != 1 {
				errChan <- fmt.Errorf("queries must return exactly one value")
				return
			}

			resultChan <- v[0].Value.String()
		} else {
			errChan <- fmt.Errorf("got unexpected result from %s: %#v", q, v)
		}
	}()

	return resultChan, errChan
}

func (p *Poller) getInitialValue(ctx context.Context, q *PromQuery) error {
	resultChan, errChan := p.query(ctx, q.String())

	select {
	case <-ctx.Done():
		return fmt.Errorf("context was cancelled")

	case result := <-resultChan:
		fmt.Printf("Initial value for %s: %s\n", q.String(), result)
		p.initialValues.Store(q.String(), result)

	case err := <-errChan:
		return err
	}

	return nil
}

func (p *Poller) Init(ctx context.Context) {
	fmt.Println("Gathering initial values...")

	wg := sync.WaitGroup{}
	for _, q := range p.queries {
		wg.Add(1)
		go func(q *PromQuery) {
			defer wg.Done()
			err := p.getInitialValue(ctx, q)
			if err != nil {
				fmt.Printf("error getting initial value for %s: %s\n", q.String(), err.Error())
				return
			}
		}(q)
	}

	wg.Wait()
}

func (p *Poller) Wait(ctx context.Context, interval time.Duration) <-chan bool {
	done := make(chan bool)

	go func() {
		p.timer = monotime.New()
		wg := sync.WaitGroup{}
		for _, q := range p.queries {
			wg.Add(1)
			go func(q *PromQuery) {
				defer wg.Done()
				startingValue, ok := p.initialValues.Load(q.String())
				if !ok {
					fmt.Println("unable to find initial value for query", q.String())
					return
				}
				targetVal, ok := startingValue.(string)
				if !ok {
					fmt.Println("initial value for query is invalid", q.String())
					return
				}

			PollLoop:
				for {
					fmt.Printf("Waiting %s before polling for %s\n", interval, q.String())
					pollTimer := time.NewTimer(interval)
					<-pollTimer.C

					resultChan, errChan := p.query(ctx, q.String())
					select {
					case <-ctx.Done():
						fmt.Println("error: timed out while waiting for metrics to normalize")
						break PollLoop

					case result := <-resultChan:
						fmt.Printf("Polling value(%s) for %s: %s\n", p.timer.Elapsed(), q.String(), result)

						if targetVal == result {
							break PollLoop
						}

					case err := <-errChan:
						fmt.Printf("error while polling %s: %s\n", q.String(), err.Error())
						continue
					}
				}

			}(q)
		}

		wg.Wait()

		done <- true
	}()

	return done
}

func NewPoller(addr string, queries []string) (*Poller, error) {
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
		client:  prometheus.NewAPI(client),
		queries: parsedQueries,
	}, nil
}
