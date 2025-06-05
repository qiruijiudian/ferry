package system

import (
	"ferry/global/orm"
	"ferry/tools"
	"fmt"
)

/*
  @Author : lanyulei
*/

type Login struct {
	Username  string `form:"UserName" json:"username" binding:"required"`
	Password  string `form:"Password" json:"password" binding:"required"`
	Code      string `form:"Code" json:"code"`
	UUID      string `form:"UUID" json:"uuid"`
	LoginType int    `form:"LoginType" json:"loginType"`
}

func (u *Login) GetUser() (user SysUser, role SysRole, e error) {

	fmt.Println("Login type:", u.LoginType)
	// 2是web短信登录 3是小程序一键登录
	if u.LoginType == 2 || u.LoginType == 3 {
		e = orm.Eloquent.Table("sys_user").Where("phone = ? ", u.Username).Find(&user).Error
		if e != nil {
			return
		}
	} else {
		e = orm.Eloquent.Table("sys_user").Where("username = ? ", u.Username).Find(&user).Error
		if e != nil {
			return
		}
	}
	// 验证密码
	if u.LoginType == 0 {
		_, e = tools.CompareHashAndPassword(user.Password, u.Password)
		if e != nil {
			return
		}
	}

	e = orm.Eloquent.Table("sys_role").Where("role_id = ? ", user.RoleId).First(&role).Error
	if e != nil {
		return
	}
	return
}
