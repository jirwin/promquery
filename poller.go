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

func (p *Poller) query(ctx context.Context, q string) (string, error) {
	val, err := p.client.Query(ctx, q, time.Now())
	if err != nil {
		return "", err
	}

	if v, ok := val.(model.Vector); ok {
		if len(v) != 1 {
			return "", fmt.Errorf("queries must return exactly one value")
		}

		return v[0].Value.String(), nil
	} else {
		return "", fmt.Errorf("got unexpected result from %s: %#v", q, v)
	}
}

func (p *Poller) getInitialValue(ctx context.Context, q *PromQuery) error {
	val, err := p.query(ctx, q.String())
	if err != nil {
		return err
	}

	fmt.Printf("Initial value for %s: %s\n", q.String(), val)
	p.initialValues.Store(q.String(), val)

	return nil
}

func (p *Poller) Init(ctx context.Context) {
	p.timer = monotime.New()

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

	//ctx1, _ := context.WithTimeout(ctx, 2 * time.Minute)

	go func() {
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

				for true {
					fmt.Printf("Waiting %s before polling for %s\n", interval, q.String())
					pollTimer := time.NewTimer(interval)
					<-pollTimer.C

					pollVal, err := p.query(ctx, q.String())
					if err != nil {
						fmt.Printf("error while polling %s: %s\n", q.String(), err.Error())
						continue
					}

					fmt.Printf("Polling value for %s: %s\n", q.String(), pollVal)

					if targetVal == pollVal {
						break
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
