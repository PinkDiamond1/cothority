package sign

import (
	"github.com/dedis/crypto/abstract"
	"github.com/dedis/cothority/lib/hashid"
	"github.com/dedis/cothority/lib/proof"
	dbg "github.com/dedis/cothority/lib/debug_lvl"
)

const FIRST_ROUND int = 1 // start counting rounds at 1

type Round struct {
								   // Message created by root. It can be empty and it will make no difference. In
								   // the case of a timestamp service however we need the timestamp generated by
								   // the round for this round . It will be included in the challenge, and then
								   // can be verified by the client
	msg            []byte
	c              abstract.Secret // round lasting challenge
	r              abstract.Secret // round lasting response

	Log            SNLog           // round lasting log structure
	HashedLog      []byte

	r_hat          abstract.Secret // aggregate of responses
	X_hat          abstract.Point  // aggregate of public keys

	Commits        []*SigningMessage
	Responses      []*SigningMessage

								   // own big merkle subtree
	MTRoot         hashid.HashId   // mt root for subtree, passed upwards
	Leaves         []hashid.HashId // leaves used to build the merkle subtre
	LeavesFrom     []string        // child names for leaves

								   // mtRoot before adding HashedLog
	LocalMTRoot    hashid.HashId

								   // merkle tree roots of children in strict order
	CMTRoots       []hashid.HashId
	CMTRootNames   []string
	Proofs         map[string]proof.Proof

								   // round-lasting public keys of children servers that did not
								   // respond to latest commit or respond phase, in subtree
	ExceptionList  []abstract.Point
								   // combined point commits of children servers in subtree
	ChildV_hat     map[string]abstract.Point
								   // combined public keys of children servers in subtree
	ChildX_hat     map[string]abstract.Point
								   // for internal verification purposes
	exceptionV_hat abstract.Point

	BackLink       hashid.HashId
	AccRound       []byte

	Vote           *Vote
								   // VoteRequest  *VoteRequest  // Vote Request vote on in the round
								   // CountedVotes *CountedVotes // CountedVotes contains a subtree's votes
}

func NewRound(suite abstract.Suite) *Round {
	round := &Round{}
	round.Commits = make([]*SigningMessage, 0)
	round.Responses = make([]*SigningMessage, 0)
	round.ExceptionList = make([]abstract.Point, 0)
	round.Log.Suite = suite
	return round
}

type RoundType int

const (
	EmptyRT RoundType = iota
	ViewChangeRT
	AddRT
	RemoveRT
	ShutdownRT
	NoOpRT
	SigningRT
)

func (rt RoundType) String() string {
	switch rt {
	case EmptyRT:
		return "empty"
	case SigningRT:
		return "signing"
	case ViewChangeRT:
		return "viewchange"
	case AddRT:
		return "add"
	case RemoveRT:
		return "remove"
	case ShutdownRT:
		return "shutdown"
	case NoOpRT:
		return "noop"
	default:
		return ""
	}
}

func RoundSetup(sn *Node, view int, am *AnnouncementMessage) error {
	// TODO: accept annoucements on old views?? linearizabiltity?
	sn.viewmu.Lock()
	// if (sn.ChangingView && am.Vote == nil) || (sn.ChangingView && am.Vote != nil && am.Vote.Vcv == nil) {
	// 	dbg.Lvl4(sn.Name(), "currently chaning view")
	// 	sn.viewmu.Unlock()
	// 	return ChangingViewError
	// }
	if sn.ChangingView && am.Vote != nil && am.Vote.Vcv == nil {
		dbg.Lvl4(sn.Name(), "currently chaning view")
		sn.viewmu.Unlock()
		return ChangingViewError
	}
	sn.viewmu.Unlock()

	sn.roundmu.Lock()
	roundNbr := am.RoundNbr
	if roundNbr <= sn.LastSeenRound {
		sn.roundmu.Unlock()
		return ErrPastRound
	}

	// make space for round type
	if len(sn.RoundTypes) <= roundNbr {
		sn.RoundTypes = append(sn.RoundTypes, make([]RoundType, max(len(sn.RoundTypes), roundNbr + 1))...)
	}
	if am.Vote == nil {
		dbg.Lvl4(roundNbr, len(sn.RoundTypes))
		sn.RoundTypes[roundNbr] = SigningRT
	} else {
		sn.RoundTypes[roundNbr] = RoundType(am.Vote.Type)
	}
	sn.roundmu.Unlock()

	// set up commit and response channels for the new round
	sn.Rounds[roundNbr] = NewRound(sn.suite)
	sn.initCommitCrypto(roundNbr)
	sn.Rounds[roundNbr].Vote = am.Vote

	// update max seen round
	sn.roundmu.Lock()
	sn.LastSeenRound = max(sn.LastSeenRound, roundNbr)
	sn.roundmu.Unlock()

	// the root is the only node that keeps track of round # internally
	if sn.IsRoot(view) {
		sn.RoundsAsRoot += 1
		// TODO: is sn.Round needed if we have LastSeenRound
		sn.Round = roundNbr

		// Create my back link to previous round
		sn.SetBackLink(roundNbr)
		// sn.SetAccountableRound(Round)
	}
	return nil
}