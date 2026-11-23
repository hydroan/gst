package serviceiamprofile

import "github.com/hydroan/gst/i18n"

// The sentences the package answers a client with.
const (
	failedToLoadProfileMsg   = "failed to load profile"
	failedToCreateProfileMsg = "failed to create profile"
	failedToUpdateProfileMsg = "failed to update profile"
)

// init registers the translations of the sentences above.
func init() {
	i18n.Register(i18n.Chinese, map[string]string{
		failedToLoadProfileMsg:   "加载个人资料失败",
		failedToCreateProfileMsg: "创建个人资料失败",
		failedToUpdateProfileMsg: "更新个人资料失败",
	})
	i18n.Register(i18n.TraditionalChinese, map[string]string{
		failedToLoadProfileMsg:   "載入個人資料失敗",
		failedToCreateProfileMsg: "建立個人資料失敗",
		failedToUpdateProfileMsg: "更新個人資料失敗",
	})
}
