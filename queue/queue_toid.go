package queue

import (
	"container/heap"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/fasthall/gochariots/info"
	"github.com/fasthall/gochariots/maintainer"
	"github.com/fasthall/gochariots/record"
	"golang.org/x/net/context"
)

var sameDCBuffered []record.TOIDRecord
var TOIDbuffered []BufferHeap

var Carry bool

func (s *Server) TOIDReceiveRecords(ctx context.Context, in *RPCRecords) (*RPCReply, error) {
	records := make([]record.TOIDRecord, len(in.GetRecords()))
	for i, ri := range in.GetRecords() {
		records[i] = record.TOIDRecord{
			Id:        ri.GetId(),
			Timestamp: ri.GetTimestamp(),
			Host:      ri.GetHost(),
			TOId:      ri.GetToid(),
			LId:       ri.GetLid(),
			Tags:      ri.GetTags(),
			Pre: record.TOIDCausality{
				Host: ri.GetHost(),
				TOId: ri.GetToid(),
			},
		}
	}
	TOIDrecordsArrival(records)
	return &RPCReply{Message: "ok"}, nil
}

func (s *Server) TOIDReceiveToken(ctx context.Context, in *RPCTOIDToken) (*RPCReply, error) {
	deferredRecords := make([]record.TOIDRecord, len(in.GetDeferredRecords()))
	for i := range deferredRecords {
		deferredRecords[i] = record.TOIDRecord{
			Id:        in.GetDeferredRecords()[i].GetId(),
			Timestamp: in.GetDeferredRecords()[i].GetTimestamp(),
			Host:      in.GetDeferredRecords()[i].GetHost(),
			TOId:      in.GetDeferredRecords()[i].GetToid(),
			LId:       in.GetDeferredRecords()[i].GetLid(),
			Tags:      in.GetDeferredRecords()[i].GetTags(),
			Pre: record.TOIDCausality{
				Host: in.GetDeferredRecords()[i].GetCausality().GetHost(),
				TOId: in.GetDeferredRecords()[i].GetCausality().GetToid(),
			},
		}
	}
	token := TOIDToken{
		MaxTOId:         in.GetMaxTOId(),
		LastLId:         in.GetLastLId(),
		DeferredRecords: deferredRecords,
	}
	if Carry {
		TokenArrivalCarryDeferred(token)
	} else {
		TokenArrivalBufferDeferred(token)
	}
	return &RPCReply{Message: "ok"}, nil
}

func (s *Server) TOIDUpdateNextQueue(ctx context.Context, in *RPCQueue) (*RPCReply, error) {
	return s.UpdateNextQueue(ctx, in)
}

func (s *Server) TOIDUpdateMaintainers(ctx context.Context, in *RPCMaintainers) (*RPCReply, error) {
	return s.UpdateMaintainers(ctx, in)
}

func (s *Server) TOIDUpdateIndexers(ctx context.Context, in *RPCIndexers) (*RPCReply, error) {
	return s.UpdateIndexers(ctx, in)
}

// Token is used by queues to ensure causality of LId assignment
type TOIDToken struct {
	MaxTOId         []uint32
	LastLId         uint32
	DeferredRecords []record.TOIDRecord
}

// InitQueue initializes the buffer and hashmap for queued records
func TOIDInitQueue(hasToken, carry bool) {
	Carry = carry
	TOIDbuffered = make([]BufferHeap, info.NumDC)
	for i := range TOIDbuffered {
		TOIDbuffered[i] = BufferHeap{}
		heap.Init(&TOIDbuffered[i])
	}
	if hasToken {
		var token TOIDToken
		token.InitToken(make([]uint32, info.NumDC), 0)
		if Carry {
			TokenArrivalCarryDeferred(token)
		} else {
			TokenArrivalBufferDeferred(token)
		}
	}
	if hasToken {
		logrus.WithField("token", true).Info("initialized")
	} else {
		logrus.WithField("token", false).Info("initialized")
	}
}

// InitToken intializes a token. The IDs info should be accuired from log maintainers
func (token *TOIDToken) InitToken(maxTOId []uint32, lastLId uint32) {
	token.MaxTOId = maxTOId
	token.LastLId = lastLId
	token.DeferredRecords = []record.TOIDRecord{}
}

// recordsArrival deals with the records received from filters
func TOIDrecordsArrival(records []record.TOIDRecord) {
	// info.LogTimestamp("recordsArrival")
	bufMutex.Lock()
	for _, r := range records {
		if r.Host == uint32(info.ID) {
			sameDCBuffered = append(sameDCBuffered, r)
		} else {
			heap.Push(&TOIDbuffered[r.Host], r)
		}
	}
	bufMutex.Unlock()
}

// TokenArrivalCarryDeferred function deals with token received.
// For each deferred records in the token, check if the current max TOId in shared log satisfies the dependency.
// If so, the deferred records are sent to the log maintainers.
func TokenArrivalCarryDeferred(token TOIDToken) {
	bufMutex.Lock()
	// append buffered records to the token in order
	for host := range TOIDbuffered {
		for TOIDbuffered[host].Len() > 0 {
			r := heap.Pop(&TOIDbuffered[host]).(record.TOIDRecord)
			token.DeferredRecords = append(token.DeferredRecords, r)
		}
	}
	token.DeferredRecords = append(token.DeferredRecords, sameDCBuffered...)
	sameDCBuffered = []record.TOIDRecord{}
	bufMutex.Unlock()
	// put the deffered records with dependency satisfied into dispatch slice

	dispatch := []record.TOIDRecord{}
	head := 0
	for _, r := range token.DeferredRecords {
		if r.Host != uint32(info.ID) {
			// default value of TOId is 0 so no need to check if the record has dependency or not
			if r.TOId == token.MaxTOId[r.Host]+1 && r.Pre.TOId <= token.MaxTOId[r.Pre.Host] {
				dispatch = append(dispatch, r)
				token.MaxTOId[r.Host] = r.TOId
			} else {
				token.DeferredRecords[head] = r
				head++
			}
		} else {
			// if it's from the same DC, TOId needs to be assigned
			if r.Pre.TOId <= token.MaxTOId[r.Pre.Host] {
				r.TOId = token.MaxTOId[r.Host] + 1
				dispatch = append(dispatch, r)
				token.MaxTOId[r.Host] = r.TOId
			} else {
				token.DeferredRecords[head] = r
				head++
			}
		}
	}
	token.DeferredRecords = token.DeferredRecords[:head]

	if len(dispatch) > 0 {
		// assign LId and send to log maintainers
		lastID := TOIDassignLId(dispatch, token.LastLId)
		token.LastLId = lastID
		toDispatch := make([][]record.TOIDRecord, len(maintainersClient))
		for _, r := range dispatch {
			id := maintainer.AssignToMaintainer(r.LId, len(maintainersClient))
			toDispatch[id] = append(toDispatch[id], r)
		}
		for id, t := range toDispatch {
			if len(t) > 0 {
				TOIDdispatchRecords(t, id)
			}
		}
	}
	go TOIDpassToken(&token)
}

// TokenArrivalBufferDeferred is similar to TokenArrivalCarryDeferred, except deferred records will be buffered rather than carried with token
func TokenArrivalBufferDeferred(token TOIDToken) {
	dispatch := []record.TOIDRecord{}
	bufMutex.Lock()
	for host := range TOIDbuffered {
		for TOIDbuffered[host].Len() > 0 {
			r := heap.Pop(&TOIDbuffered[host]).(record.TOIDRecord)
			if r.TOId == token.MaxTOId[r.Host]+1 && r.Pre.TOId <= token.MaxTOId[r.Pre.Host] {
				dispatch = append(dispatch, r)
				token.MaxTOId[r.Host] = r.TOId
			} else {
				heap.Push(&TOIDbuffered[host], r)
				break
			}
		}
	}
	head := 0
	for _, r := range sameDCBuffered {
		if r.Pre.TOId <= token.MaxTOId[r.Pre.Host] {
			r.TOId = token.MaxTOId[r.Host] + 1
			dispatch = append(dispatch, r)
			token.MaxTOId[r.Host] = r.TOId
		} else {
			sameDCBuffered[head] = r
			head++
		}
	}
	sameDCBuffered = sameDCBuffered[:head]
	bufMutex.Unlock()
	// put the deffered records with dependency satisfied into dispatch slice
	if len(dispatch) > 0 {
		// assign LId and send to log maintainers
		lastID := TOIDassignLId(dispatch, token.LastLId)
		token.LastLId = lastID
		toDispatch := make([][]record.TOIDRecord, len(maintainersClient))
		for _, r := range dispatch {
			id := maintainer.AssignToMaintainer(r.LId, len(maintainersClient))
			toDispatch[id] = append(toDispatch[id], r)
		}
		for id, t := range toDispatch {
			if len(t) > 0 {
				TOIDdispatchRecords(t, id)
			}
		}
	}
	go TOIDpassToken(&token)
}

// assignLId assigns LId to all the records to be sent to log maintainers
// return the last LId assigned
func TOIDassignLId(records []record.TOIDRecord, lastLId uint32) uint32 {
	for i := range records {
		lastLId++
		records[i].LId = lastLId
	}
	return lastLId
}

// passToken sends the token to the next queue in the ring
func TOIDpassToken(token *TOIDToken) {
	time.Sleep(100 * time.Millisecond)
	if nextQueueHost == "" {
		if Carry {
			TokenArrivalCarryDeferred(*token)
		} else {
			TokenArrivalBufferDeferred(*token)
		}
	} else {
		rpcDeferredRecords := make([]*RPCRecord, len(token.DeferredRecords))
		for i := range rpcDeferredRecords {
			rpcDeferredRecords[i] = &RPCRecord{
				Id:        token.DeferredRecords[i].Id,
				Timestamp: token.DeferredRecords[i].Timestamp,
				Host:      token.DeferredRecords[i].Host,
				Toid:      token.DeferredRecords[i].TOId,
				Lid:       token.DeferredRecords[i].LId,
				Tags:      token.DeferredRecords[i].Tags,
				Causality: &RPCCausality{
					Host: token.DeferredRecords[i].Pre.Host,
					Toid: token.DeferredRecords[i].Pre.TOId,
				},
			}
		}
		rpcTOIdToken := RPCTOIDToken{
			MaxTOId:         token.MaxTOId,
			LastLId:         token.LastLId,
			DeferredRecords: rpcDeferredRecords,
		}
		nextQueueClient.TOIDReceiveToken(context.Background(), &rpcTOIdToken)
	}
}

// dispatchRecords sends the ready records to log maintainers
func TOIDdispatchRecords(records []record.TOIDRecord, maintainerID int) {
	// info.LogTimestamp("dispatchRecords")
	rpcRecords := maintainer.RPCRecords{
		Records: make([]*maintainer.RPCRecord, len(records)),
	}
	for i, r := range records {
		tmp := maintainer.RPCRecord{
			Id:        r.Id,
			Timestamp: r.Timestamp,
			Host:      r.Host,
			Toid:      r.TOId,
			Lid:       r.LId,
			Tags:      r.Tags,
			Causality: &maintainer.RPCCausality{
				Host: r.Pre.Host,
				Toid: r.Pre.TOId,
			},
		}
		rpcRecords.Records[i] = &tmp
	}
	_, err := maintainersClient[maintainerID].TOIDReceiveRecords(context.Background(), &rpcRecords)
	if err != nil {
		logrus.WithField("id", maintainerID).Error("failed to connect to maintainer")
	} else {
		logrus.WithFields(logrus.Fields{"records": records, "id": maintainerID}).Debug("sent the records to maintainer")
	}
	// log.Printf("TIMESTAMP %s:record in queue %s\n", info.GetName(), time.Since(lastTime))
}
