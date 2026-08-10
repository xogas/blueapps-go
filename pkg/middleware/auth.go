/*
 * TencentBlueKing is pleased to support the open source community by making
 * 蓝鲸智云 - Go 开发框架 (BlueKing - Go Framework) available.
 * Copyright (C) 2017 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 *
 *	https://opensource.org/licenses/MIT
 *
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * We undertake not to change the open source license (MIT license) applicable
 * to the current version of the project delivered to anyone in the future.
 */

package middleware

import (
	"context"
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"

	"github.com/TencentBlueKing/blueapps-go/pkg/account"
	"github.com/TencentBlueKing/blueapps-go/pkg/common"
	log "github.com/TencentBlueKing/blueapps-go/pkg/logging"
	"github.com/TencentBlueKing/blueapps-go/pkg/utils/ginx"
)

// UserAuth 进行用户身份认证，并将用户信息注入到 context 中
func UserAuth(authBackends []account.AuthBackend) gin.HandlerFunc {
	// 如果没有提供 AuthBackend，则直接返回 401 并提示需要配置 AuthBackend
	if len(authBackends) == 0 {
		return func(c *gin.Context) {
			c.String(http.StatusUnauthorized, "Auth backend required!")
			c.Abort()
		}
	}

	return func(c *gin.Context) {
		ctx := c.Request.Context()
		session := sessions.Default(c)

		var err error
		var userToken string
		var userInfo *account.UserInfo

		// 逐个 AuthBackend 尝试获取用户信息
		for _, backend := range authBackends {
			userToken, err = backend.GetUserToken(c)
			// 没有获取到用户凭证信息 -> 401 -> 让用户通过页面登录
			if err != nil {
				log.Infof(ctx, "backend %s get user token failed: %v", backend.Name(), err)
				continue
			}

			// 用户凭证信息与 Session 中的一致 -> 直接通过
			if userToken == session.Get(common.UserTokenKey) {
				setUserIDInContext(c, session.Get(common.UserIDKey).(string))
				c.Next()
				return
			}

			// 尝试获取用户信息
			userInfo, err = backend.GetUserInfo(c, userToken)
			if err != nil {
				log.Infof(ctx, "backend %s get user info failed: %v", backend.Name(), err)
				continue
			}
			// 任意 AuthBackend 获取到用户信息 -> 不再继续尝试
			break
		}

		// 没有获取到用户凭证信息 -> 401 -> 让用户通过页面登录
		if userToken == "" || userInfo == nil || userInfo.ID == "" {
			// 最后一个 AuthBackend 来提供登录重定向链接
			backend := authBackends[len(authBackends)-1]
			// 重定向到登录页面
			redirectToLoginPage(c, backend)
			return
		}

		// 获取到用户凭证信息 -> 设置 context & session -> 通过
		setUserIDInContext(c, userInfo.ID)
		session.Set(common.UserIDKey, userInfo.ID)
		session.Set(common.UserTokenKey, userInfo.Token)
		session.Set(common.UserAuthSourceKey, userInfo.AuthSource)
		_ = session.Save()
		c.Next()
	}
}

// 重定向到登录页面
func redirectToLoginPage(c *gin.Context, backend account.AuthBackend) {
	// 获取回调地址，如果有显示指定 Referer 则使用，否则使用当前页面
	callbackUrl := lo.Ternary(c.Request.Referer() != "", c.Request.Referer(), getFullRequestURL(c))
	ginH := gin.H{
		"authType": backend.Name(),
		"loginUrl": backend.GetLoginUrl(callbackUrl),
		"lang":     ginx.GetLang(c),
	}

	c.HTML(http.StatusUnauthorized, "401.html", ginH)
	c.Abort()
}

// getFullRequestURL 获取完整的请求 URL（包含 scheme）
func getFullRequestURL(c *gin.Context) string {
	scheme := "http"
	// 优先从 X-Forwarded-Proto 获取（反向代理场景）
	if proto := c.GetHeader("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if c.Request.TLS != nil {
		// 直连场景，通过 TLS 判断
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host + c.Request.URL.String()
}

// 在 Context 中设置用户信息
func setUserIDInContext(c *gin.Context, userID string) {
	ctx := c.Request.Context()
	// 在 context 中设置 UserID
	c.Request = c.Request.WithContext(
		context.WithValue(ctx, common.UserIDCtxKey, userID),
	)
	// 在 gin.Context 中设置 UserID
	ginx.SetUserID(c, userID)
}
