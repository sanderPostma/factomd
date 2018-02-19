package election

import (
	"fmt"

	"github.com/FactomProject/electiontesting/imessage"
	"github.com/FactomProject/electiontesting/messages"
	. "github.com/FactomProject/electiontesting/primitives"
	"github.com/FactomProject/electiontesting/volunteercontrol"
)

var _ = fmt.Println

// RoutingElection is just an election that returns msgs for broadcasting
type RoutingElection struct {
	*Election
}

func NewRoutingElection(e *Election) *RoutingElection {
	r := new(RoutingElection)
	r.Election = e

	return r
}

func (r *RoutingElection) Execute(msg imessage.IMessage) (imessage.IMessage, bool) {
	resp, ch := r.Election.execute(msg)
	if resp == nil && ch {
		return msg, ch
	}

	if resp == nil && r.Election.CurrentVote.Level > 0 {
		return &r.Election.CurrentVote, false
	}

	return resp, ch
}

type Election struct {
	// Level 0 volunteer votes map[vol]map[leader]msg
	VolunteerVotes map[Identity]map[Identity]*messages.VoteMessage

	// Indexed by volunteer
	VolunteerControls map[Identity]*volunteercontrol.VolunteerControl

	CurrentLevel int
	CurrentVote  messages.LeaderLevelMessage
	Self         Identity
	AuthSet
	ProcessListLocation

	MsgListIn  []*messages.LeaderLevelMessage
	MsgListOut []*messages.LeaderLevelMessage

	Display *Display
	// If I have committed to an answer and found enough to finish Election
	Committed bool

	// Some statistical info
	TotalMessages int

	// Each time I vote for the same vol in the next level
	CommitmentTally int
}

func NewElection(self Identity, authset AuthSet, loc ProcessListLocation) *Election {
	e := new(Election)
	e.VolunteerVotes = make(map[Identity]map[Identity]*messages.VoteMessage)
	e.VolunteerControls = make(map[Identity]*volunteercontrol.VolunteerControl)
	e.Self = self
	e.AuthSet = authset
	// Majority level starts at 1
	e.CurrentLevel = 1
	e.CurrentVote.Rank = -1
	e.CurrentVote.VolunteerPriority = -1
	e.MsgListIn = make([]*messages.LeaderLevelMessage, 0)
	e.MsgListOut = make([]*messages.LeaderLevelMessage, 0)

	// Used to determine volunteer priority
	e.ProcessListLocation = loc
	return e
}

func (a *Election) Copy() *Election {
	b := NewElection(a.Self, a.AuthSet.Copy(), a.ProcessListLocation)
	b.TotalMessages = a.TotalMessages
	b.CommitmentTally = a.CommitmentTally

	for k, _ := range a.VolunteerVotes {
		b.VolunteerVotes[k] = make(map[Identity]*messages.VoteMessage)
		for k2, v2 := range a.VolunteerVotes[k] {
			b.VolunteerVotes[k][k2] = v2.Copy()
		}
	}

	for k, v := range a.VolunteerControls {
		b.VolunteerControls[k] = v.Copy()
	}

	b.CurrentLevel = a.CurrentLevel
	b.CurrentVote = *(a.CurrentVote.Copy())
	if a.Display == nil {
		b.Display = nil
	} else {
		b.Display = a.Display.Copy(b)
		b.Display.Global = a.Display.Global.Copy(b)
	}
	b.Committed = a.Committed
	b.MsgListIn = make([]*messages.LeaderLevelMessage, len(a.MsgListIn))

	for i, v := range a.MsgListIn {
		b.MsgListIn[i] = v.Copy()
	}

	b.MsgListOut = make([]*messages.LeaderLevelMessage, len(a.MsgListOut))
	for i, v := range a.MsgListOut {
		b.MsgListOut[i] = v.Copy()
	}

	return b
}

// Returns false if it finds a similarity that we don't want
func (a *Election) IsDifferent(b *Election) bool {
	if fmt.Sprintf("%p", a.MsgListIn) == fmt.Sprintf("%p", b.MsgListIn) {
		if len(a.MsgListIn)+len(b.MsgListIn) != 0 {
			return false
		}
	}

	if fmt.Sprintf("%p", a.MsgListOut) == fmt.Sprintf("%p", b.MsgListOut) {
		if len(a.MsgListOut)+len(b.MsgListOut) != 0 {
			return false
		}
	}

	return true
}

// AddDisplay takes a global tracker. Send nil if you don't care about
// a global state
func (e *Election) AddDisplay(global *Display) *Display {
	e.Display = NewDisplay(e, global)
	return e.Display
}

func (e *Election) updateCurrentVote(new messages.LeaderLevelMessage) {
	if new.VolunteerPriority == e.CurrentVote.VolunteerPriority {
		if e.CurrentVote.Rank+1 == new.Rank {
			e.CommitmentTally++
			e.CurrentVote = new
			return
		}
	}
	e.CommitmentTally = 1
	e.CurrentVote = new
}

func (e *Election) PrintMessages() string {
	str := fmt.Sprintf("-- In -- (%p)\n", e.MsgListIn)
	for i, m := range e.MsgListIn {
		str += fmt.Sprintf("%d %s\n", i, e.Display.FormatMessage(m))
	}
	str += fmt.Sprintf("-- Out -- (%p)\n", e.MsgListOut)
	for i, m := range e.MsgListOut {
		str += fmt.Sprintf("%d %s\n", i, e.Display.FormatMessage(m))
	}
	return str
}

func (e *Election) Execute(msg imessage.IMessage) (imessage.IMessage, bool) {
	resp, c := e.execute(msg)
	if l, ok := msg.(*messages.LeaderLevelMessage); ok {
		e.MsgListOut = append(e.MsgListOut, l)
	}
	return resp, c
}

func (e *Election) execute(msg imessage.IMessage) (imessage.IMessage, bool) {
	e.TotalMessages++
	e.executeDisplay(msg)

	// We are done, never use anything else
	if e.Committed {
		return nil, false
	}

	switch msg.(type) {
	case *messages.LeaderLevelMessage:
		return e.executeLeaderLevelMessage(msg.(*messages.LeaderLevelMessage))
	case *messages.VolunteerMessage:
		vol := msg.(*messages.VolunteerMessage)
		if e.VolunteerVotes[vol.Signer] == nil {
			e.VolunteerVotes[vol.Signer] = make(map[Identity]*messages.VoteMessage)
		}

		// already voted
		if _, ok := e.VolunteerVotes[vol.Signer][e.Self]; ok {
			fmt.Println("Votes!")
			return nil, false
		}

		vote := messages.NewVoteMessage(*vol, e.Self)
		e.VolunteerVotes[vol.Signer][e.Self] = &vote

		msg, _ := e.Execute(&vote)
		if ll, ok := msg.(*messages.LeaderLevelMessage); ok {
			for _, v := range e.VolunteerVotes[vol.Signer] {
				ll.VoteMessages = append(ll.VoteMessages, v)
			}
			return ll, true
		}

		return &vote, true
	case *messages.VoteMessage:
		// Colecting these allows us to issue out 0.#
		vote := msg.(*messages.VoteMessage)
		vol := vote.Volunteer.Signer

		if e.VolunteerVotes[vol] == nil {
			e.VolunteerVotes[vol] = make(map[Identity]*messages.VoteMessage)
		}
		e.VolunteerVotes[vol][vote.Signer] = vote
		e.executeDisplay(vote)
		if len(e.VolunteerVotes[vote.Volunteer.Signer]) >= e.Majority() {
			// We have a majority of level 0 votes and can issue a rank 0 LeaderLevel Message

			// No current vote, so send!
			if e.CurrentVote.Rank == -1 {
				goto SendRank0
			}

			// If we have a rank 1+, we will not issue a rank 0
			if e.CurrentVote.Rank > 0 {
				return nil, false // forward our answer again
			}

			// If we have a rank 0, and higher priority volunteer (or same, don't vote again)
			if e.CurrentVote.VolunteerPriority >= e.getVolunteerPriority(vol) {
				return nil, false // forward our answer again
			}

		SendRank0:
			ll := messages.NewLeaderLevelMessage(e.Self, 0, e.CurrentLevel, vote.Volunteer)
			ll.VolunteerPriority = e.getVolunteerPriority(vote.Volunteer.Signer)
			for _, v := range e.VolunteerVotes[vote.Volunteer.Signer] {
				ll.VoteMessages = append(ll.VoteMessages, v)
			}

			e.CurrentLevel++
			e.updateCurrentVote(ll)

			//e.executeDisplay(&ll)
			ret, _ := e.Execute(&ll)
			// The response could be a better vote. If it is, send that out instead
			if ret != nil {
				if l, ok := ret.(*messages.LeaderLevelMessage); ok {
					if (&ll).Less(l) {
						return l, true
					}
				}
			}

			return &ll, true
		}
	}
	return nil, false
}

func (e *Election) stack(msg imessage.IMessage) {

}

func (e *Election) getVolunteerPriority(vol Identity) int {
	return e.GetVolunteerPriority(vol, e.ProcessListLocation)
}

func (e *Election) executeDisplay(msg imessage.IMessage) {
	if e.Display != nil {
		e.Display.Execute(msg)
	}
}

func (e *Election) executeLeaderLevelMessage(msg *messages.LeaderLevelMessage) (imessage.IMessage, bool) {
	if e.VolunteerControls[msg.VolunteerMessage.Signer] == nil {
		e.VolunteerControls[msg.VolunteerMessage.Signer] = volunteercontrol.NewVolunteerControl(e.Self, e.AuthSet)
	}

	e.MsgListIn = append(e.MsgListIn, msg)

	if msg.Level <= 0 {
		panic("level <= 0 should never happen")
	}

	// We may actually get a majority vote at rank 0 from this. We need to account for that
	// Did a vote change happen?
	voteChange := false
	var rank0Vote *messages.LeaderLevelMessage
	// Votes exist, so we can add these to our vote map
	if len(msg.VoteMessages) > 0 {
		// If we get a new current vote from this, we will get a vote change. In which case we will
		// send out our current vote if nothing is better
		for _, v := range msg.VoteMessages {
			resp, v := e.Execute(v)
			if resp != nil && rank0Vote != nil {
				vl, castok := resp.(*messages.LeaderLevelMessage)
				if castok {
					rank0Vote = vl
				}
			}
			voteChange = v || voteChange
		}
	}

	// Add their info to our map

	// All responses if not nil is a message created by us
	res, change := e.VolunteerControls[msg.VolunteerMessage.Signer].Execute(msg)
	if res != nil {
		// Adding things to the display
		ll, ok := res.(*messages.LeaderLevelMessage)
		if ok {
			// We need to add the justifications to our map
			if e.Display != nil {
				for _, jl := range ll.Justification {
					e.Display.Execute(jl)
				}
				for _, jv := range ll.VoteMessages {
					e.Display.Execute(jv)
				}
			}
		}

		// We need to set the volunteer priority for comparing
		ll.VolunteerPriority = e.getVolunteerPriority(ll.VolunteerMessage.Signer)

		// We have a new vote we might be able to cast
		if e.CurrentVote.Less(ll) {
			// This vote is better than our current, let's pass it out.
			if ll.Rank >= e.CurrentLevel {
				// We cannot issue a rank 2 on level 2. It has to be on level 3
				ll.Level = ll.Rank + 1
				e.CurrentLevel = ll.Rank + 2
			} else {
				// Set level to our level, increment
				ll.Level = e.CurrentLevel
				e.CurrentLevel++
			}

			// This vote may change our next vote. First we need to check
			// if this is our LAST vote
			e.updateCurrentVote(*ll)

			e.commitIfLast(ll)
			if e.Committed {
				return ll, true
			}

			// This is not our last vote, let's check if it triggers a new vote
			msg, _ := e.Execute(ll)
			if msg != nil {
				if l2, ok := msg.(*messages.LeaderLevelMessage); ok {
					return e.commitIfLast(l2), true
				}
			}

			return ll, true
		}

		// We decided not to send the new msg
		if ll.Level < 0 {
			return nil, change
		}
	}

	if rank0Vote != nil {
		return nil, voteChange || change
	}

	// No msg generated
	return nil, voteChange || change
}

func (e *Election) commitIfLast(msg *messages.LeaderLevelMessage) *messages.LeaderLevelMessage {
	// If commit is true, then we are done. Return the EOM
	// commit := e.CommitmentIndicator.ShouldICommit(msg)
	if e.CommitmentTally > 2 { //commit {
		e.Committed = true
		msg.Committed = true
		e.Display.Execute(msg)
		return msg
	}
	return msg
}

func (e *Election) VolunteerControlString() string {
	str := "VolunteerControls\n"
	if e.Display == nil {
		return "No display\n"
	}

	vcs := make([]string, 0)

	for i, v := range e.VolunteerControls {
		line := fmt.Sprintf("(%d) ", e.getVolunteerPriority(i))
		if e.VolunteerControls[i] == nil {
			line += "nil"
		} else {
			votes := ""
			sep := ""
			for _, vo := range v.Votes {
				votes += sep + e.Display.FormatMessage(vo)
				sep = ","
			}
			line += votes
		}

		vcs = append(vcs, line)
	}

	for _, l := range vcs {
		str += l + "\n"
	}

	return str
}
