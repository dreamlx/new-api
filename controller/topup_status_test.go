package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpStatusTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	model.InitColumnsForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedTopUpStatusUser(t *testing.T, db *gorm.DB, id int, username string) *model.User {
	t.Helper()
	user := &model.User{
		Id:       id,
		Username: username,
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    100,
		Group:    "default",
		AffCode:  username + "_aff",
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func seedTopUpRow(t *testing.T, db *gorm.DB, userId int, tradeNo, paymentMethod, status string, createTime int64) *model.TopUp {
	t.Helper()
	topUp := &model.TopUp{
		UserId:         userId,
		Amount:         100,
		Money:          14.50,
		TradeNo:        tradeNo,
		PaymentMethod:  paymentMethod,
		CreateTime:     createTime,
		Status:         status,
		PayAmountCents: 1450,
		ExpireTime:     createTime + 1800,
	}
	require.NoError(t, db.Create(topUp).Error)
	return topUp
}

func callGetTopUpStatus(t *testing.T, userId int, tradeNo string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", userId)
	q := ""
	if tradeNo != "" {
		q = "?trade_no=" + tradeNo
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/status"+q, nil)
	GetTopUpStatus(ctx)
	return recorder
}

func TestGetTopUpStatusHappyPath(t *testing.T) {
	db := setupTopUpStatusTestDB(t)
	user := seedTopUpStatusUser(t, db, 1, "owner_user")
	topUp := seedTopUpRow(t, db, user.Id, "USR1NO_status_ok", PaymentMethodAlipay, common.TopUpStatusPending, 1000)

	recorder := callGetTopUpStatus(t, user.Id, topUp.TradeNo)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"message":"success"`)
	require.Contains(t, body, `"trade_no":"USR1NO_status_ok"`)
	require.Contains(t, body, `"status":"pending"`)
	require.Contains(t, body, `"payment_method":"alipay"`)
	require.Contains(t, body, `"amount":100`)
	require.Contains(t, body, `"money":14.5`)
}

func TestGetTopUpStatusForeignTradeNoReturns404(t *testing.T) {
	db := setupTopUpStatusTestDB(t)
	owner := seedTopUpStatusUser(t, db, 2, "real_owner")
	attacker := seedTopUpStatusUser(t, db, 1, "attacker_user")
	topUp := seedTopUpRow(t, db, owner.Id, "USR2NO_secret", PaymentMethodAlipay, common.TopUpStatusPending, 1000)

	recorder := callGetTopUpStatus(t, attacker.Id, topUp.TradeNo)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"message":"error"`)
	require.Contains(t, body, "订单不存在")
	require.NotContains(t, body, `"trade_no":"USR2NO_secret"`,
		"foreign trade_no must not be echoed back; that would confirm existence to a probing user")
}

func TestGetTopUpStatusUnknownTradeNoReturns404(t *testing.T) {
	db := setupTopUpStatusTestDB(t)
	user := seedTopUpStatusUser(t, db, 1, "owner_user")
	_ = db

	recorder := callGetTopUpStatus(t, user.Id, "USR1NO_does_not_exist")

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Body.String(), "订单不存在")
}

func TestGetTopUpStatusEmptyTradeNo(t *testing.T) {
	setupTopUpStatusTestDB(t)

	recorder := callGetTopUpStatus(t, 1, "")

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `"message":"error"`)
	require.Contains(t, body, "参数错误")
}
