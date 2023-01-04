package storage

import (
	"fmt"
	"github.com/andyj29/wannabet/internal/domain/common"
	"github.com/andyj29/wannabet/internal/infrastructure/logger"
	"github.com/jetbasrawi/go.geteventstore"
	"net/url"
	"reflect"
	"time"
)

type EventStore struct {
	client        *goes.Client
	eventRegistry map[string]func() (common.Event, map[string]string)
}

func NewEventStore(addr string) *EventStore {
	client, err := goes.NewClient(nil, addr)
	if err != nil {
		logger.InfraLogger.Fatalf("failed to establish new event store http connection")
	}

	return &EventStore{
		client: client,
	}
}

func (es *EventStore) Append(event common.Event, meta map[string]string) error {
	streamWriter := es.client.NewStreamWriter(event.GetAggregateID())
	newEvent := goes.NewEvent(goes.NewUUID(), event.GetEventType(), event, meta)
	return streamWriter.Append(nil, newEvent)
}

func (es *EventStore) ReadAll(stream string, callback func(common.Event, bool) error) error {
	streamReader := es.client.NewStreamReader(stream)
	for streamReader.Next() {
		if err := streamReader.Err(); err != nil {
			switch err.(type) {
			case *url.Error, *goes.ErrTemporarilyUnavailable:
				logger.InfraLogger.Infof("The event store server is not ready at the moment: %v. Attempt to retry after 10 seconds", err)
				<-time.After(time.Duration(10) * time.Second)

			case *goes.ErrNotFound:
				logger.InfraLogger.Errorf("Could not find stream with this ID: %v")
				return fmt.Errorf("stream with this ID not found")

			case *goes.ErrUnauthorized:
				logger.InfraLogger.Fatalf("Read is not authorized for this stream: %v", err)

			case *goes.ErrNoMoreEvents:
				return nil
			}
		}

		var event common.Event
		var meta interface{}

		if f, ok := es.eventRegistry[streamReader.EventResponse().Event.EventType]; ok {
			event, meta = f()
			if err := streamReader.Scan(&event, &meta); err != nil {
				logger.InfraLogger.Fatalf(err.Error())
			}
			if err := callback(event, false); err != nil {
				return err
			}
		}
	}
	return nil
}

type Repository[T common.AggregateRoot] struct {
	es *EventStore
}

func NewRepository[T common.AggregateRoot](eventStore *EventStore) *Repository[T] {
	return &Repository[T]{
		es: eventStore,
	}
}

func (r *Repository[T]) Load(aggregateID string) (T, error) {
	var deserializedAggregate T
	initNilAggregatePtr(&deserializedAggregate)
	if err := r.es.ReadAll(aggregateID, deserializedAggregate.When); err != nil {
		var nilAggregate T
		return nilAggregate, err
	}
	return deserializedAggregate, nil
}

func (r *Repository[T]) Save(aggregate T) error {
	changes := aggregate.GetChanges()
	for _, change := range changes {
		if err := r.es.Append(change, nil); err != nil {
			logger.InfraLogger.Errorf(err.Error())
			return err
		}
	}
	return nil
}

func initNilAggregatePtr(dp interface{}) {
	target := reflect.ValueOf(dp).Elem()
	if reflect.Indirect(target).IsValid() {
		return
	}

	aggregateType := target.Type().Elem()
	target.Set(reflect.New(aggregateType))
}