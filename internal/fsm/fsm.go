package fsm

import (
	"context"
	"sync"

	"github.com/TrixiS/goram"
	"github.com/TrixiS/goram/handlers"
)

const StateCtxKey = "stateData"

type FSM struct {
	data map[int64]*StateContext
	mu   sync.RWMutex
}

type StateContext struct {
	State string
	Data  any
}

func New() *FSM {
	fsm := FSM{
		data: map[int64]*StateContext{},
	}

	return &fsm
}

func (fsm *FSM) InitState(id int64, state StateContext) {
	fsm.mu.Lock()
	fsm.data[id] = &state
	fsm.mu.Unlock()
}

func (fsm *FSM) ClearState(id int64) {
	fsm.mu.Lock()
	delete(fsm.data, id)
	fsm.mu.Unlock()
}

func (fsm *FSM) FilterMessage(state string) handlers.Filter[*goram.Message] {
	return func(ctx context.Context, bot *goram.Bot, message *goram.Message, data handlers.Data) (bool, error) {
		if message.From == nil {
			return false, nil
		}

		fsm.mu.RLock()
		stateCtx, ok := fsm.data[message.From.ID]

		if !ok || stateCtx.State != state {
			fsm.mu.RUnlock()
			return false, nil
		}

		fsm.mu.RUnlock()

		data[StateCtxKey] = stateCtx
		return true, nil
	}
}
