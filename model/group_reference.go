package model

import "gorm.io/gorm"

type GroupReferences struct {
	Users  int64
	Tokens int64
}

func FindGroupReferences(db *gorm.DB, groups []string) (GroupReferences, error) {
	references := GroupReferences{}
	if len(groups) == 0 {
		return references, nil
	}
	if err := db.Model(&User{}).Where(commonGroupCol+" IN ?", groups).Count(&references.Users).Error; err != nil {
		return references, err
	}
	if err := db.Model(&Token{}).Where(commonGroupCol+" IN ?", groups).Count(&references.Tokens).Error; err != nil {
		return references, err
	}
	return references, nil
}
