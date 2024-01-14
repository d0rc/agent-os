package message_store

import (
	"github.com/d0rc/agent-os/engines"
	"strings"
	"sync"
)

type NodeID string
type TrajectorySignature string

type SemanticTrajectory []NodeID

type SemanticNode struct {
	ID                      NodeID
	IncomingContexts        map[TrajectorySignature]SemanticTrajectory // the ways this node gets created....!
	IncomingContextsWeights map[TrajectorySignature]uint64
	ResultingNodes          map[TrajectorySignature]map[NodeID]uint64
	Lock                    sync.RWMutex
}

type SemanticSpace struct {
	nodes     map[NodeID]*SemanticNode
	nodesLock sync.RWMutex
}

func NewSemanticSpace() *SemanticSpace {
	return &SemanticSpace{
		nodes:     make(map[NodeID]*SemanticNode),
		nodesLock: sync.RWMutex{},
	}
}

// AddMessage - to add a new message we have to pass
// both message and the path to it
func (space *SemanticSpace) AddMessage(path []string, message *engines.Message) {
	nodeId := NodeID(*message.ID)
	space.nodesLock.RLock()
	node, exists := space.nodes[nodeId]
	space.nodesLock.RUnlock()
	if !exists {
		space.nodesLock.Lock()
		node, exists = space.nodes[nodeId]
		if !exists {
			node = &SemanticNode{
				ID:                      nodeId,
				IncomingContexts:        make(map[TrajectorySignature]SemanticTrajectory),
				IncomingContextsWeights: make(map[TrajectorySignature]uint64),
				ResultingNodes:          make(map[TrajectorySignature]map[NodeID]uint64),
				Lock:                    sync.RWMutex{},
			}
			space.nodes[nodeId] = node
		}
		space.nodesLock.Unlock()
	}

	space.nodesLock.RLock()
	node = space.nodes[nodeId]
	space.nodesLock.RUnlock()

	trajectory := ToTrajectory(path)
	trajectorySignature := GetTrajectorySignature(trajectory)

	node.Lock.Lock()
	node.IncomingContexts[trajectorySignature] = trajectory
	node.IncomingContextsWeights[trajectorySignature]++
	node.Lock.Unlock()

	parentNodeId := NodeID(path[len(path)-1])
	parentNodeTrajectorySignature := GetTrajectorySignature(ToTrajectory(path[:len(path)-1]))

	space.nodesLock.Lock()
	parentNode, exists := space.nodes[parentNodeId]
	space.nodesLock.Unlock()

	if exists {
		parentNode.Lock.Lock()
		if _, exists := parentNode.ResultingNodes[parentNodeTrajectorySignature]; !exists {
			parentNode.ResultingNodes[parentNodeTrajectorySignature] = make(map[NodeID]uint64)
		}
		parentNode.ResultingNodes[parentNodeTrajectorySignature][node.ID]++
		parentNode.Lock.Unlock()
	}
}

func GetTrajectorySignature(trajectory SemanticTrajectory) TrajectorySignature {
	// join all strings
	builder := strings.Builder{}
	for _, el := range trajectory {
		builder.WriteString(string(el))
		builder.WriteString(",")
	}
	return TrajectorySignature(engines.GenerateMessageId(builder.String()))
}

func ToTrajectory(path []string) SemanticTrajectory {
	result := make(SemanticTrajectory, len(path))
	for idx, segment := range path {
		result[idx] = NodeID(segment)
	}

	return result
}
