package domain

type User struct {
	BaseUUID
	Username     string `json:"username" gorm:"uniqueIndex;not null"`
	Email        string `json:"email" gorm:"uniqueIndex;not null"`
	PasswordHash string `json:"-" gorm:"not null"`

	// Relationships
	Reviews []Review `json:"reviews,omitempty" gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}
