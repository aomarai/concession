package domain

import (
	"bytes"
	"time"

	"github.com/google/uuid"
)

type FriendshipStatus string

const (
	FriendshipPending  FriendshipStatus = "pending"
	FriendshipAccepted FriendshipStatus = "accepted"
)

// Friendship links two users. A request is a pending Friendship that becomes
// accepted when the other user agrees. There is at most one row per pair of
// users, whichever of them sent the request: the pair is stored in a canonical
// order (see OrderedPair), so a duplicate or reverse request hits the unique
// index. Declining, cancelling and unfriending delete the row.
type Friendship struct {
	BaseUUID
	UserAID     uuid.UUID        `json:"user_a_id" gorm:"column:user_a_id;type:uuid;not null;index;uniqueIndex:idx_friendship_pair"`
	UserBID     uuid.UUID        `json:"user_b_id" gorm:"column:user_b_id;type:uuid;not null;index;uniqueIndex:idx_friendship_pair"`
	RequestedBy uuid.UUID        `json:"requested_by" gorm:"type:uuid;not null"`
	Status      FriendshipStatus `json:"status" gorm:"type:varchar(20);default:'pending';not null"`
	AcceptedAt  *time.Time       `json:"accepted_at,omitempty"`
}

// OrderedPair returns the two IDs in the canonical (byte-wise ascending)
// order used for Friendship.UserAID and UserBID.
func OrderedPair(a, b uuid.UUID) (uuid.UUID, uuid.UUID) {
	if bytes.Compare(a[:], b[:]) <= 0 {
		return a, b
	}
	return b, a
}

// Other returns the member of the pair that is not userID.
func (f Friendship) Other(userID uuid.UUID) uuid.UUID {
	if f.UserAID == userID {
		return f.UserBID
	}
	return f.UserAID
}

// Addressee is the user the request was sent to.
func (f Friendship) Addressee() uuid.UUID { return f.Other(f.RequestedBy) }
