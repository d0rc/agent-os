package message_store

import (
	"fmt"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/tools"
	"strings"
	"sync"
	"time"
)

type MessageID string
type Trajectory []MessageID
type TrajectoryID string

type SemanticSpace struct {
	trajectories     map[TrajectoryID]*Trajectory
	lock             sync.RWMutex
	newRequests      []*Trajectory
	pendingRequests  map[TrajectoryID]uint64
	messages         map[MessageID]*engines.Message
	growthFactor     int
	nPendingRequests int
	waiters          []chan struct{}
}

func NewSemanticSpace(growthFactor int) *SemanticSpace {
	return &SemanticSpace{
		trajectories:     make(map[TrajectoryID]*Trajectory),
		lock:             sync.RWMutex{},
		newRequests:      make([]*Trajectory, 0),
		pendingRequests:  make(map[TrajectoryID]uint64),
		nPendingRequests: 0,
		growthFactor:     growthFactor,
		messages:         make(map[MessageID]*engines.Message),
		waiters:          make([]chan struct{}, 0),
	}
}

func (space *SemanticSpace) GetComputeRequests(maxRequests, maxPendingRequests int) []*Trajectory {
	if space.nPendingRequests >= maxPendingRequests {
		return nil
	}

	result := make([]*Trajectory, 0, maxRequests)
	space.lock.Lock()

	// let's take first maxRequests from newRequests
	// removing them from newRequests
	for i := 0; i < maxRequests && len(space.newRequests) > 0; i++ {
		result = append(result, space.newRequests[0])
		space.newRequests = space.newRequests[1:]
	}

	for _, trajectory := range result {
		space.pendingRequests[GenerateTrajectoryID(*trajectory)]++
		space.nPendingRequests++
	}
	space.lock.Unlock()

	return result

}

func (space *SemanticSpace) CancelPendingRequest(trajectoryID TrajectoryID) {
	space.lock.Lock()
	pendingReqs, exists := space.pendingRequests[trajectoryID]
	if exists && pendingReqs > 0 {
		// drop first element from pendingReqs
		space.pendingRequests[trajectoryID]--
		space.nPendingRequests--

		for _, waiter := range space.waiters {
			waiter <- struct{}{}
		}
		space.waiters = make([]chan struct{}, 0)
	} else {
		// fmt.Printf("can't find a request to close")
	}

	space.lock.Unlock()
}

func (space *SemanticSpace) Wait() {
	space.lock.Lock()
	if space.nPendingRequests > 0 {
		waitChan := make(chan struct{}, 1)
		space.waiters = append(space.waiters, waitChan)
		space.lock.Unlock()
		<-waitChan
	} else {
		space.lock.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
}

func (space *SemanticSpace) AddMessage(trajectoryId *TrajectoryID, message *engines.Message) error {
	space.lock.Lock()
	space.messages[(MessageID)(*message.ID)] = message
	space.lock.Unlock()

	if trajectoryId == nil {
		newTrajectory := Trajectory{MessageID(*message.ID)}
		newTrajectoryId := GenerateTrajectoryID(newTrajectory)
		space.lock.Lock()
		space.trajectories[newTrajectoryId] = &newTrajectory
		if message.Role == engines.ChatRoleSystem || message.Role == engines.ChatRoleUser {
			space.newRequests = append(space.newRequests, tools.Replicate(&newTrajectory, space.growthFactor)...)
		}
		space.lock.Unlock()
		return nil
	} else {
		// since we have a trajectoryId, let's check if it exists
		space.lock.RLock()
		trajectory, exists := space.trajectories[*trajectoryId]
		space.lock.RUnlock()
		if !exists {
			return fmt.Errorf("trajectory %s does not exist", *trajectoryId)
		}

		// if we got here - chances, some pending requests have been fulfilled
		// let's try to guess which one
		if message.Role == engines.ChatRoleAssistant {
			space.CancelPendingRequest(*trajectoryId)
		}

		// if we got here, we have a trajectoryId, let's add the message to it
		space.lock.Lock()
		newTrajectory := append(*trajectory, MessageID(*message.ID))
		newTrajectoryId := GenerateTrajectoryID(newTrajectory)
		_, exists = space.trajectories[newTrajectoryId]
		if !exists {
			space.trajectories[newTrajectoryId] = &newTrajectory
			if message.Role == engines.ChatRoleSystem || message.Role == engines.ChatRoleUser {
				space.newRequests = append(space.newRequests, tools.Replicate(&newTrajectory, space.growthFactor)...)
			}
		}
		space.lock.Unlock()

		return nil
	}
}

func (space *SemanticSpace) TrajectoryToMessages(request *Trajectory) []*engines.Message {
	result := make([]*engines.Message, 0, len(*request))
	space.lock.RLock()
	for _, id := range *request {
		message := space.messages[id]
		result = append(result, message)
	}
	space.lock.RUnlock()
	return result
}

func GenerateTrajectoryID(trajectory Trajectory) TrajectoryID {
	builder := strings.Builder{}
	for _, messageID := range trajectory {
		builder.Write([]byte(messageID))
	}

	return TrajectoryID(engines.GenerateMessageId(builder.String()))
}

func (space *SemanticSpace) GetNextTrajectoryID(sourceTrajectoryId TrajectoryID, direction MessageID) (TrajectoryID, error) {
	space.lock.RLock()
	trajectory, exists := space.trajectories[sourceTrajectoryId]
	space.lock.RUnlock()
	if !exists {
		return "", fmt.Errorf("source trajectory %s does not exist", sourceTrajectoryId)
	}

	newTrajectory := append(*trajectory, direction)
	newTrajectoryId := GenerateTrajectoryID(newTrajectory)

	return newTrajectoryId, nil
}
