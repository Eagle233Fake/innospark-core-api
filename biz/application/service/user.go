package service

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/wire"
	"github.com/xh-polaris/innospark-core-api/biz/adaptor"
	"github.com/xh-polaris/innospark-core-api/biz/application/dto/basic"
	"github.com/xh-polaris/innospark-core-api/biz/application/dto/core_api"
	"github.com/xh-polaris/innospark-core-api/biz/infra/config"
	"github.com/xh-polaris/innospark-core-api/biz/infra/cst"
	"github.com/xh-polaris/innospark-core-api/biz/infra/mapper/user"
	"github.com/xh-polaris/innospark-core-api/biz/infra/util"
	"github.com/xh-polaris/innospark-core-api/biz/infra/util/httpx"
	"github.com/xh-polaris/innospark-core-api/pkg/errorx"
	"github.com/xh-polaris/innospark-core-api/pkg/logs"
	"github.com/xh-polaris/innospark-core-api/types/errno"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type IUserService interface {
	SendVerifyCode(ctx context.Context, req *core_api.SendVerifyCodeReq) (*core_api.SendVerifyCodeResp, error)
	Register(ctx context.Context, req *core_api.BasicUserRegisterReq) (*core_api.BasicUserRegisterResp, error)
	Login(ctx context.Context, req *core_api.BasicUserLoginReq) (*core_api.BasicUserLoginResp, error)
	ResetPassword(ctx context.Context, req *core_api.BasicUserResetPasswordReq) (*core_api.BasicUserResetPasswordResp, error)
	UpdateProfile(ctx context.Context, req *core_api.BasicUserUpdateProfileReq) (*core_api.BasicUserUpdateProfileResp, error)
	ThirdPartyLogin(ctx context.Context, req *core_api.ThirdPartyLoginReq) (*core_api.ThirdPartyLoginResp, error)
}

type UserService struct {
	UserMapper user.MongoMapper
}

var UserServiceSet = wire.NewSet(
	wire.Struct(new(UserService), "*"),
	wire.Bind(new(IUserService), new(*UserService)),
)

func (u *UserService) SendVerifyCode(ctx context.Context, req *core_api.SendVerifyCodeReq) (*core_api.SendVerifyCodeResp, error) {
	c := config.GetConfig()
	header := http.Header{}
	header.Set("content-type", "application/json")
	if c.State != "test" {
		header.Set("X-Xh-Env", "test")
	}
	body := map[string]any{
		"authType": req.AuthType,
		"authId":   req.AuthId,
		"expire":   300,
		"cause":    "passport",
		"app":      map[string]any{"name": "InnoSpark"},
	}

	url := config.GetConfig().SynapseURL + "/system/send_verify_code"
	resp, err := httpx.GetHttpClient().Post(url, header, body)
	if err != nil {
		return nil, errorx.WrapByCode(err, errno.SynapseErrCode, errorx.KV("url", url))
	}
	if resp["code"].(float64) != 0 {
		return &core_api.SendVerifyCodeResp{
			Resp: &basic.Response{
				Code: int32(resp["code"].(float64)),
				Msg:  resp["msg"].(string),
			},
		}, nil
	}
	return &core_api.SendVerifyCodeResp{
		Resp: util.Success(),
	}, nil
}

func (u *UserService) Register(ctx context.Context, req *core_api.BasicUserRegisterReq) (*core_api.BasicUserRegisterResp, error) {
	c := config.GetConfig()
	header := http.Header{}
	header.Set("content-type", "application/json")
	if c.State != "test" {
		header.Set("X-Xh-Env", "test")
	}
	body := map[string]any{
		"authType": req.AuthType,
		"authId":   req.AuthId,
		"verify":   req.Verify,
		"password": req.Password,
		"app":      map[string]any{"name": "InnoSpark"},
	}

	url := config.GetConfig().SynapseURL + "/basic_user/register"
	resp, err := httpx.GetHttpClient().Post(url, header, body)
	if err != nil {
		return nil, errorx.WrapByCode(err, errno.ErrRegister)
	}
	if resp["code"].(float64) != 0 {
		return &core_api.BasicUserRegisterResp{
			Resp: &basic.Response{
				Code: int32(resp["code"].(float64)),
				Msg:  resp["msg"].(string),
			},
		}, nil
	}
	return &core_api.BasicUserRegisterResp{
		Resp:  util.Success(),
		Token: resp["token"].(string),
	}, nil
}

func (u *UserService) Login(ctx context.Context, req *core_api.BasicUserLoginReq) (*core_api.BasicUserLoginResp, error) {
	c := config.GetConfig()
	header := http.Header{}
	header.Set("content-type", "application/json")
	if c.State != "test" {
		header.Set("X-Xh-Env", "test")
	}
	body := map[string]any{
		"authType": req.AuthType,
		"authId":   req.AuthId,
		"verify":   req.Verify,
		"app":      map[string]any{"name": "InnoSpark"},
	}

	url := config.GetConfig().SynapseURL + "/basic_user/login"
	resp, err := httpx.GetHttpClient().Post(url, header, body)
	if err != nil {
		return nil, errorx.WrapByCode(err, errno.ErrLogin)
	}
	if resp["code"].(float64) != 0 {
		return &core_api.BasicUserLoginResp{
			Resp: &basic.Response{
				Code: int32(resp["code"].(float64)),
				Msg:  resp["msg"].(string),
			},
		}, nil
	}
	var id, phone string
	if basicUser, ok := resp["basicUser"].(map[string]any); ok {
		if id, ok = basicUser["basicUserId"].(string); ok && id != "" {
			if strings.HasPrefix(req.AuthType, "phone-") {
				phone = req.AuthId
			}
			if _, err = u.UserMapper.FindOrCreateUser(ctx, id, phone, true); err != nil {
				return nil, errorx.WrapByCode(err, errno.ErrLogin)
			}
			return &core_api.BasicUserLoginResp{
				Resp:  util.Success(),
				Token: resp["token"].(string),
				New:   resp["new"].(bool),
			}, nil
		}
	}
	return nil, errorx.New(errno.ErrLogin)
}

func (u *UserService) ResetPassword(ctx context.Context, req *core_api.BasicUserResetPasswordReq) (*core_api.BasicUserResetPasswordResp, error) {
	c := config.GetConfig()
	rc, err := adaptor.ExtractContext(ctx)
	if err != nil {
		return nil, errorx.WrapByCode(err, errno.UnAuthErrCode)
	}
	header := http.Header{}
	header.Set("content-type", "application/json")
	header.Set("Authorization", string(rc.GetHeader("Authorization")))
	if c.State != "test" {
		header.Set("X-Xh-Env", "test")
	}
	body := map[string]any{
		"newPassword": req.NewPassword,
		"app":         map[string]any{"name": "InnoSpark"},
	}

	url := config.GetConfig().SynapseURL + "/basic_user/reset_password"
	resp, err := httpx.GetHttpClient().Post(url, header, body)
	if err != nil {
		return nil, errorx.WrapByCode(err, errno.SynapseErrCode, errorx.KV("url", url))
	}
	if resp["code"].(float64) != 0 {
		return &core_api.BasicUserResetPasswordResp{
			Resp: &basic.Response{
				Code: int32(resp["code"].(float64)),
				Msg:  resp["msg"].(string),
			},
		}, nil
	}
	return &core_api.BasicUserResetPasswordResp{
		Resp: util.Success(),
	}, nil
}

func (u *UserService) UpdateProfile(ctx context.Context, req *core_api.BasicUserUpdateProfileReq) (*core_api.BasicUserUpdateProfileResp, error) {
	// 鉴权
	uid, err := adaptor.ExtractUserId(ctx)
	if err != nil {
		logs.Errorf("extract user id error: %s", errorx.ErrorWithoutStack(err))
		return nil, errorx.WrapByCode(err, errno.UnAuthErrCode)
	}

	objUid, err := primitive.ObjectIDFromHex(uid)
	if err != nil {
		return nil, err
	}

	// 构建更新字段
	update := make(bson.M)
	if req.Username != nil {
		update[cst.Name] = *req.Username
	}
	if req.Avatar != nil {
		update[cst.Avatar] = *req.Avatar
	}

	// 一次性更新所有字段
	if len(update) > 0 {
		if err = u.UserMapper.UpdateField(ctx, objUid, update); err != nil {
			logs.Errorf("update user profile error: %s", errorx.ErrorWithoutStack(err))
			return nil, errorx.WrapByCode(err, errno.ErrUpdateUserField)
		}
	}

	return &core_api.BasicUserUpdateProfileResp{Resp: util.Success()}, nil
}

func (u *UserService) ThirdPartyLogin(ctx context.Context, req *core_api.ThirdPartyLoginReq) (*core_api.ThirdPartyLoginResp, error) {
	c := config.GetConfig()
	header := http.Header{}
	header.Set("content-type", "application/json")
	if c.State != "test" {
		header.Set("X-Xh-Env", "test")
	}
	body := map[string]any{
		"thirdparty": req.Thirdparty,
		"ticket":     req.Ticket,
	}

	url := config.GetConfig().SynapseURL + "/thirdparty/login"
	resp, err := httpx.GetHttpClient().Post(url, header, body)
	if err != nil {
		return nil, errorx.WrapByCode(err, errno.SynapseErrCode, errorx.KV("url", url))
	}
	if resp["code"].(float64) != 0 {
		return &core_api.ThirdPartyLoginResp{
			Resp: &basic.Response{
				Code: int32(resp["code"].(float64)),
				Msg:  resp["msg"].(string),
			},
		}, nil
	}
	var id string
	if basicUser, ok := resp["basicUser"].(map[string]any); ok {
		if id, ok = basicUser["basicUserId"].(string); ok && id != "" {
			if _, err = u.UserMapper.FindOrCreateUser(ctx, id, "第三方用户", true); err != nil {
				return nil, errorx.WrapByCode(err, errno.ErrLogin)
			}
			return &core_api.ThirdPartyLoginResp{
				Resp:  util.Success(),
				Token: resp["token"].(string),
				New:   false,
			}, nil
		}
	}
	return &core_api.ThirdPartyLoginResp{
		Resp:  util.Success(),
		Token: resp["token"].(string),
		New:   false,
	}, nil
}
