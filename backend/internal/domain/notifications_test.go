package domain

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupNotificationTestDB spins up an in-memory SQLite DB for Notification
// tests only. FK constraint creation is disabled during migration since
// Notification.Actor references User, and we don't need User's exact
// fields to test Notification itself — we only ever store a bare UUID in
// ActorID/UserID, never a persisted User row.
func setupNotificationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&Notification{}); err != nil {
		t.Fatalf("failed to migrate Notification: %v", err)
	}

	return db
}

func TestNotificationTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  NotificationType
		want string
	}{
		{"NotificationWatchlistInvite", NotificationWatchlistInvite, "watchlist_invite"},
		{"NotificationItemAdded", NotificationItemAdded, "item_added"},
		{"NotificationFriendRequest", NotificationFriendRequest, "friend_request"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

func TestCreateNotification(t *testing.T) {
	db := setupNotificationTestDB(t)

	userID := uuid.New()
	actorID := uuid.New()

	n := Notification{
		UserID:  userID,
		ActorID: actorID,
		Type:    NotificationFriendRequest,
		Message: "Alex sent you a friend request",
		LinkURL: "/friends/requests",
	}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("unexpected error creating notification: %v", err)
	}

	if n.ID == uuid.Nil {
		t.Error("expected BaseUUID's BeforeCreate hook to populate ID, got uuid.Nil")
	}

	var fetched Notification
	if err := db.First(&fetched, "id = ?", n.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching notification: %v", err)
	}
	if fetched.UserID != userID {
		t.Errorf("expected UserID %v, got %v", userID, fetched.UserID)
	}
	if fetched.ActorID != actorID {
		t.Errorf("expected ActorID %v, got %v", actorID, fetched.ActorID)
	}
	if fetched.Type != NotificationFriendRequest {
		t.Errorf("expected Type %q, got %q", NotificationFriendRequest, fetched.Type)
	}
	if fetched.Message != "Alex sent you a friend request" {
		t.Errorf("unexpected Message: %q", fetched.Message)
	}
}

func TestNotificationIsReadDefaultsFalse(t *testing.T) {
	db := setupNotificationTestDB(t)

	// Deliberately not setting IsRead, to confirm the `default:false`
	// column default applies (Go's zero value for bool is already false,
	// so this also implicitly checks that the column isn't inserted as
	// something the DB coerces into true, e.g. via a bad default tag).
	n := Notification{
		UserID:  uuid.New(),
		ActorID: uuid.New(),
		Type:    NotificationItemAdded,
		Message: "New item added to your watchlist",
	}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("unexpected error creating notification: %v", err)
	}

	var fetched Notification
	if err := db.First(&fetched, "id = ?", n.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching notification: %v", err)
	}
	if fetched.IsRead {
		t.Error("expected IsRead to default to false")
	}
}

func TestMarkNotificationAsRead(t *testing.T) {
	db := setupNotificationTestDB(t)

	n := Notification{
		UserID:  uuid.New(),
		ActorID: uuid.New(),
		Type:    NotificationWatchlistInvite,
		Message: "You were invited to a watchlist",
	}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("unexpected error creating notification: %v", err)
	}

	if err := db.Model(&n).Update("is_read", true).Error; err != nil {
		t.Fatalf("unexpected error marking notification as read: %v", err)
	}

	var fetched Notification
	if err := db.First(&fetched, "id = ?", n.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching notification: %v", err)
	}
	if !fetched.IsRead {
		t.Error("expected IsRead to be true after update")
	}
}

func TestListUnreadNotificationsForUser(t *testing.T) {
	db := setupNotificationTestDB(t)

	userID := uuid.New()
	otherUserID := uuid.New()

	unread := Notification{UserID: userID, ActorID: uuid.New(), Type: NotificationItemAdded, Message: "unread for target user"}
	read := Notification{UserID: userID, ActorID: uuid.New(), Type: NotificationItemAdded, Message: "already read", IsRead: true}
	otherUsers := Notification{UserID: otherUserID, ActorID: uuid.New(), Type: NotificationItemAdded, Message: "belongs to someone else"}

	for _, notif := range []*Notification{&unread, &read, &otherUsers} {
		if err := db.Create(notif).Error; err != nil {
			t.Fatalf("unexpected error seeding notification: %v", err)
		}
	}

	var results []Notification
	if err := db.Where("user_id = ? AND is_read = ?", userID, false).Find(&results).Error; err != nil {
		t.Fatalf("unexpected error querying unread notifications: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 unread notification for user, got %d", len(results))
	}
	if results[0].ID != unread.ID {
		t.Errorf("expected the unread notification to be returned, got a different one")
	}
}

func TestNotificationRequiresMessage(t *testing.T) {
	// Message has gorm:"not null", but Go's zero value for string is "",
	// which SQLite accepts as a valid (non-NULL) value — an empty string
	// is not NULL. So this "not null" tag alone won't reject an empty
	// message; it only guards against an actual NULL, which isn't
	// reachable by populating the struct with its zero value. This test
	// documents that rather than asserting a rejection that won't happen.
	db := setupNotificationTestDB(t)

	n := Notification{
		UserID:  uuid.New(),
		ActorID: uuid.New(),
		Type:    NotificationItemAdded,
		Message: "",
	}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("unexpected error: an empty (but non-NULL) Message should be accepted by the not-null constraint, got %v", err)
	}
}
