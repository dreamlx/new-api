package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupRefundTestDB mirrors the alipay/wxpay test harness but is used by the
// refund tests so they remain independent from the per-provider test files.
func setupRefundTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColumnsForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}))

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// seedRefundUser creates a non-root regular user that owns the TopUp.
func seedRefundUser(t *testing.T, db *gorm.DB) *model.User {
	t.Helper()
	user := &model.User{
		Id:       1,
		Username: "refund_user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Quota:    500000,
		Group:    "default",
		AffCode:  "USER_AFF",
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

// seedRefundAdmin creates a root admin with id=2.
func seedRefundAdmin(t *testing.T, db *gorm.DB) *model.User {
	t.Helper()
	admin := &model.User{
		Id:       2,
		Username: "refund_admin",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Role:     common.RoleRootUser,
		Group:    "default",
		AffCode:  "ADMIN_AFF",
	}
	require.NoError(t, db.Create(admin).Error)
	return admin
}

// seedRefundSuccessTopUp inserts a successfully-completed TopUp ready to be
// refunded. PayAmountCents=10000 corresponds to 100.00 CNY; QuotaGranted=
// 50000 represents the quota previously credited and to be revoked.
func seedRefundSuccessTopUp(t *testing.T, db *gorm.DB, tradeNo string, method string, userId int) *model.TopUp {
	t.Helper()
	topUp := &model.TopUp{
		UserId:         userId,
		Amount:         100,
		Money:          100.0,
		TradeNo:        tradeNo,
		PaymentMethod:  method,
		CreateTime:     time.Now().Unix() - 600,
		CompleteTime:   time.Now().Unix() - 500,
		Status:         common.TopUpStatusSuccess,
		PayAmountCents: 10000,
		Currency:       "CNY",
		QuotaGranted:   50000,
	}
	require.NoError(t, db.Create(topUp).Error)
	return topUp
}

func newRefundPrepareCtx(t *testing.T, body string, role int, adminId int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("role", role)
	ctx.Set("id", adminId)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/topup/refund/prepare",
		bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

// TestRefundPrepareRejectsNonRoot ensures admins (role=10) cannot mint a
// confirm_token. Refund authority is root-only.
func TestRefundPrepareRejectsNonRoot(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	seedRefundSuccessTopUp(t, db, "refund_prep_001", PaymentMethodAlipay, user.Id)

	ctx, rec := newRefundPrepareCtx(t, `{"trade_no":"refund_prep_001"}`, common.RoleAdminUser, 99)

	RefundPrepare(ctx)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), "error")
}

func TestRefundPrepareRejectsMissingTradeNo(t *testing.T) {
	setupRefundTestDB(t)

	ctx, rec := newRefundPrepareCtx(t, `{}`, common.RoleRootUser, 2)
	RefundPrepare(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "error")
}

func TestRefundPrepareRejectsUnknownTradeNo(t *testing.T) {
	setupRefundTestDB(t)

	ctx, rec := newRefundPrepareCtx(t, `{"trade_no":"does_not_exist"}`, common.RoleRootUser, 2)
	RefundPrepare(ctx)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestRefundPrepareRejectsNonSuccessOrder(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	topUp := &model.TopUp{
		UserId:        user.Id,
		Amount:        100,
		TradeNo:       "refund_prep_pending",
		PaymentMethod: PaymentMethodAlipay,
		CreateTime:    time.Now().Unix(),
		Status:        common.TopUpStatusPending, // not refundable
	}
	require.NoError(t, db.Create(topUp).Error)

	ctx, rec := newRefundPrepareCtx(t, `{"trade_no":"refund_prep_pending"}`, common.RoleRootUser, 2)
	RefundPrepare(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "error")
	require.Contains(t, rec.Body.String(), "未支付")
}

func TestRefundPrepareRejectsAlreadyRefunded(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	topUp := seedRefundSuccessTopUp(t, db, "refund_prep_already", PaymentMethodAlipay, user.Id)
	topUp.RefundStatus = common.RefundStatusSuccess
	require.NoError(t, db.Save(topUp).Error)

	ctx, rec := newRefundPrepareCtx(t, `{"trade_no":"refund_prep_already"}`, common.RoleRootUser, 2)
	RefundPrepare(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "error")
	require.Contains(t, rec.Body.String(), "已发起退款")
}

func TestRefundPrepareRejectsRefundPending(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	topUp := seedRefundSuccessTopUp(t, db, "refund_prep_in_progress", PaymentMethodAlipay, user.Id)
	topUp.RefundStatus = common.RefundStatusPending
	require.NoError(t, db.Save(topUp).Error)

	ctx, rec := newRefundPrepareCtx(t, `{"trade_no":"refund_prep_in_progress"}`, common.RoleRootUser, 2)
	RefundPrepare(ctx)

	require.Contains(t, rec.Body.String(), "error")
}

func TestRefundPrepareHappyPath(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	topUp := seedRefundSuccessTopUp(t, db, "refund_prep_ok", PaymentMethodAlipay, user.Id)

	before := time.Now().Unix()
	ctx, rec := newRefundPrepareCtx(t, `{"trade_no":"refund_prep_ok"}`, common.RoleRootUser, admin.Id)
	RefundPrepare(ctx)
	after := time.Now().Unix()

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "success")
	require.Contains(t, body, "confirm_token")
	require.Contains(t, body, topUp.TradeNo)

	// Sanity check the embedded expires_at and signature by re-verifying.
	var resp struct {
		Data struct {
			ConfirmToken string `json:"confirm_token"`
			ExpiresAt    int64  `json:"expires_at"`
			TradeNo      string `json:"trade_no"`
			UserId       int    `json:"user_id"`
			Username     string `json:"username"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))
	require.GreaterOrEqual(t, resp.Data.ExpiresAt, before+int64(refundConfirmTokenTTL.Seconds())-2)
	require.LessOrEqual(t, resp.Data.ExpiresAt, after+int64(refundConfirmTokenTTL.Seconds())+2)
	require.Equal(t, topUp.TradeNo, resp.Data.TradeNo)
	require.Equal(t, user.Id, resp.Data.UserId)
	require.Equal(t, user.Username, resp.Data.Username)

	require.NoError(t, verifyConfirmToken(resp.Data.ConfirmToken, topUp.TradeNo, admin.Id, time.Now().Unix()))
}

// TestRefundPrepareTokenBindsToAdmin ensures a token minted for admin A
// fails verification when presented by admin B. This is the core security
// property of the confirm_token: HMAC over admin_id makes the token
// non-portable across operators.
func TestRefundPrepareTokenBindsToAdmin(t *testing.T) {
	db := setupRefundTestDB(t)
	user := seedRefundUser(t, db)
	admin := seedRefundAdmin(t, db)
	seedRefundSuccessTopUp(t, db, "refund_prep_bind", PaymentMethodAlipay, user.Id)

	ctx, rec := newRefundPrepareCtx(t, `{"trade_no":"refund_prep_bind"}`, common.RoleRootUser, admin.Id)
	RefundPrepare(ctx)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			ConfirmToken string `json:"confirm_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &resp))

	// Same trade_no but a different admin: must fail.
	err := verifyConfirmToken(resp.Data.ConfirmToken, "refund_prep_bind", admin.Id+1, time.Now().Unix())
	require.Error(t, err)

	// Same trade_no AND same admin: ok.
	require.NoError(t, verifyConfirmToken(resp.Data.ConfirmToken, "refund_prep_bind", admin.Id, time.Now().Unix()))
}

func TestVerifyConfirmTokenExpired(t *testing.T) {
	expired := time.Now().Add(-time.Minute).Unix()
	token := signConfirmToken("USRtrade", 2, expired)
	err := verifyConfirmToken(token, "USRtrade", 2, time.Now().Unix())
	require.Error(t, err)
}

func TestVerifyConfirmTokenTamperedTradeNo(t *testing.T) {
	exp := time.Now().Add(time.Minute).Unix()
	token := signConfirmToken("USRtrade", 2, exp)
	err := verifyConfirmToken(token, "USRother", 2, time.Now().Unix())
	require.Error(t, err)
}
