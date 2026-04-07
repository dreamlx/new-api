package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// SetWisemodelRouter 配置Wisemodel MaaS平台API路由
// 路由前缀: /api/wisemodel
// 鉴权: ExternalAPIAuth (env: WISEMODEL_API_TOKEN)
func SetWisemodelRouter(router *gin.Engine) {
	wisemodelGroup := router.Group("/api/wisemodel")

	wisemodelGroup.Use(middleware.ExternalAPIAuth("WISEMODEL_API_TOKEN"))
	{
		// 用户管理接口
		wisemodelGroup.POST("/user/bind", controller.WisemodelBind)
		wisemodelGroup.POST("/user/delete_wisemodel_key", controller.DeleteWisemodelKey)
		wisemodelGroup.POST("/user/update_wisemodel_key", controller.UpdateWisemodelKey)
		wisemodelGroup.POST("/user/update_phone", controller.UpdatePhone)
		wisemodelGroup.POST("/user/delete_wisemodel_user", controller.DeleteWisemodelUser)

		// 资源包管理接口
		wisemodelGroup.POST("/orders/record", controller.CreateOrder)
		wisemodelGroup.POST("/user/package_usage", controller.GetPackageUsage)
	}
}
