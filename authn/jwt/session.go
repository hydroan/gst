package jwt

import (
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/model"
)

// func setToken(accessToken, refreshToken string, s *model.Session) {
// 	if s == nil {
// 		return
// 	}
// 	accessTokenCache.Add(accessToken, s)
// 	refreshTokenCache.Add(refreshToken, s)
// 	database.Database[*model.Session]().Update(s)
// }
//
// func removeToken(accessToken, refreshToken string) {
// 	sessions := make([]*model.Session, 0)
// 	if len(accessToken) > 0 {
// 		accessTokenCache.Remove(accessToken)
// 		if err := database.Database[*model.Session]().WithLimit(-1).WithQuery(&model.Session{AccessToken: accessToken}).WithSelect("id").List(&sessions); err == nil {
// 			database.Database[*model.Session]().WithPurge().Delete(sessions...)
// 		}
// 	}
//
// 	if len(refreshToken) > 0 {
// 		refreshTokenCache.Remove(refreshToken)
// 		if err := database.Database[*model.Session]().WithLimit(-1).WithQuery(&model.Session{RefreshToken: refreshToken}).WithSelect("id").List(&sessions); err == nil {
// 			database.Database[*model.Session]().WithPurge().Delete(sessions...)
// 		}
// 	}
// }
//
// func GetAccessToken(accessToken string) (*model.Session, bool) {
// 	return accessTokenCache.Get(accessToken)
// }
//
// func GetRefreshToken(refreshToken string) (*model.Session, bool) {
// 	return refreshTokenCache.Get(refreshToken)
// }

func setSession(userID string, s *model.Session) {
	if len(userID) == 0 || s == nil {
		return
	}
	_ = database.Database[*model.Session](nil).Update(s)
	// sessionCache.Add 必须在 database.Update 之后, 因为它的ID会在 database.Database 之后生成
	sessionCache.Add(userID, s)
}

func GetSession(userID string) (*model.Session, bool) {
	// TODO: database
	return sessionCache.Get(userID)
}

func removeSession(userID string) {
	sessionCache.Remove(userID)
	sessions := make([]*model.Session, 0)
	if err := database.Database[*model.Session](nil).WithLimit(-1).WithSelect("id").WithQuery(&model.Session{UserID: userID}).List(&sessions); err == nil {
		_ = database.Database[*model.Session](nil).WithPurge().Delete(sessions...)
	}
}
