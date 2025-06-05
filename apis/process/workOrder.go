package process

import (
	"bytes"
	"encoding/json"
	"errors"
	"ferry/global/orm"
	"ferry/models/process"
	"ferry/models/system"
	"ferry/pkg/notify"
	"ferry/pkg/pagination"
	"ferry/pkg/service"
	"ferry/tools"
	"ferry/tools/app"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

var wechatTokenCache struct {
	sync.RWMutex
	AccessToken string
	ExpiresAt   int64 // 过期时间戳（毫秒级）
}

/*
@Author : lanyulei
*/
type WeChatConfig struct {
	AppID     string `json:"appid"`
	AppSecret string `json:"appSecret"`
}

type ProjectListResult struct {
	TplID         uint            `gorm:"column:tpl_id" json:"tplId"`
	WorkOrderID   uint            `gorm:"column:work_order_id" json:"workOrderId"`
	CreateTime    time.Time       `gorm:"column:create_time" json:"createTime"`
	UpdateTime    time.Time       `gorm:"column:update_time" json:"updateTime"`
	WorkOrder     int             `gorm:"column:work_order" json:"-"`
	FormStructure json.RawMessage `gorm:"column:form_structure" json:"formStructure"`
	FormData      json.RawMessage `gorm:"column:form_data" json:"formData"`
	Title         string          `json:"title"`
	Priority      int             `json:"priority"`
	Process       int             `gorm:"column:process" json:"processId"`
	State         json.RawMessage `json:"state"`
	IsEnd         int             `gorm:"column:is_end" json:"isEnd"`
	RelatedPerson json.RawMessage `gorm:"column:related_person" json:"relatedPerson"`
	Creator       int             `json:"creator"`
	Classify      int             `json:"classify"`
}

// 流程结构包括节点，流转和模版
func ProcessStructure(c *gin.Context) {
	processId := c.DefaultQuery("processId", "")
	if processId == "" {
		app.Error(c, -1, errors.New("参数不正确，请确定参数processId是否传递"), "")
		return
	}
	workOrderId := c.DefaultQuery("workOrderId", "0")
	if workOrderId == "" {
		app.Error(c, -1, errors.New("参数不正确，请确定参数workOrderId是否传递"), "")
		return
	}
	workOrderIdInt, _ := strconv.Atoi(workOrderId)
	processIdInt, _ := strconv.Atoi(processId)
	result, err := service.ProcessStructure(c, processIdInt, workOrderIdInt)
	if err != nil {
		app.Error(c, -1, err, "")
		return
	}

	if workOrderIdInt != 0 {
		currentState := result["workOrder"].(service.WorkOrderData).CurrentState
		userAuthority, err := service.JudgeUserAuthority(c, workOrderIdInt, currentState)
		if err != nil {
			app.Error(c, -1, err, fmt.Sprintf("判断用户是否有权限失败，%v", err.Error()))
			return
		}
		result["userAuthority"] = userAuthority
	}

	app.OK(c, result, "数据获取成功")
}

// 新建工单
func CreateWorkOrder(c *gin.Context) {

	err := service.CreateWorkOrder(c)
	if err != nil {
		app.Error(c, -1, err, "")
		return
	}

	app.OK(c, "", "成功提交工单申请")
}

// 工单列表
func WorkOrderList(c *gin.Context) {
	/*
		1. 待办工单
		2. 我创建的
		3. 我相关的
		4. 所有工单
	*/

	var (
		result      interface{}
		err         error
		classifyInt int
	)

	classify := c.DefaultQuery("classify", "")
	if classify == "" {
		app.Error(c, -1, errors.New("参数错误，请确认classify是否传递"), "")
		return
	}

	classifyInt, _ = strconv.Atoi(classify)
	w := service.WorkOrder{
		Classify: classifyInt,
		GinObj:   c,
	}
	result, err = w.WorkOrderList()
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("查询工单数据失败，%v", err.Error()))
		return
	}

	app.OK(c, result, "")
}

// 处理工单
func ProcessWorkOrder(c *gin.Context) {
	var (
		err           error
		userAuthority bool
		handle        service.Handle
		params        struct {
			Tasks          []string
			TargetState    string                   `json:"target_state"`    // 目标状态
			SourceState    string                   `json:"source_state"`    // 源状态
			WorkOrderId    int                      `json:"work_order_id"`   // 工单ID
			Circulation    string                   `json:"circulation"`     // 流转ID
			FlowProperties int                      `json:"flow_properties"` // 流转类型 0 拒绝，1 同意，2 其他
			Remarks        string                   `json:"remarks"`         // 处理的备注信息
			Tpls           []map[string]interface{} `json:"tpls"`            // 表单数据
			IsExecTask     bool                     `json:"is_exec_task"`    // 是否执行任务
		}
	)

	err = c.ShouldBind(&params)
	if err != nil {
		app.Error(c, -1, err, "")
		return
	}

	// 处理工单
	userAuthority, err = service.JudgeUserAuthority(c, params.WorkOrderId, params.SourceState)
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("判断用户是否有权限失败，%v", err.Error()))
		return
	}
	if !userAuthority {
		app.Error(c, -1, errors.New("当前用户没有权限进行此操作"), "")
		return
	}

	err = handle.HandleWorkOrder(
		c,
		params.WorkOrderId,    // 工单ID
		params.Tasks,          // 任务列表
		params.TargetState,    // 目标节点
		params.SourceState,    // 源节点
		params.Circulation,    // 流转标题
		params.FlowProperties, // 流转属性
		params.Remarks,        // 备注信息
		params.Tpls,           // 工单数据更新
		params.IsExecTask,     // 是否执行任务
	)
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("处理工单失败，%v", err.Error()))
		return
	}

	app.OK(c, nil, "工单处理完成")
}

// 结束工单
func UnityWorkOrder(c *gin.Context) {
	var (
		err           error
		workOrderId   string
		workOrderInfo process.WorkOrderInfo
		userInfo      system.SysUser
	)

	workOrderId = c.DefaultQuery("work_oroder_id", "")
	if workOrderId == "" {
		app.Error(c, -1, errors.New("参数不正确，work_oroder_id"), "")
		return
	}

	tx := orm.Eloquent.Begin()

	// 查询工单信息
	err = tx.Model(&workOrderInfo).
		Where("id = ?", workOrderId).
		Find(&workOrderInfo).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("查询工单失败，%v", err.Error()))
		return
	}
	if workOrderInfo.IsEnd == 1 {
		app.Error(c, -1, errors.New("工单已结束"), "")
		return
	}

	// 更新工单状态
	err = tx.Model(&process.WorkOrderInfo{}).
		Where("id = ?", workOrderId).
		Update("is_end", 1).
		Error
	if err != nil {
		tx.Rollback()
		app.Error(c, -1, err, fmt.Sprintf("结束工单失败，%v", err.Error()))
		return
	}

	// 获取当前用户信息
	err = tx.Model(&userInfo).
		Where("user_id = ?", tools.GetUserId(c)).
		Find(&userInfo).Error
	if err != nil {
		tx.Rollback()
		app.Error(c, -1, err, fmt.Sprintf("当前用户查询失败，%v", err.Error()))
		return
	}

	// 写入历史
	tx.Create(&process.CirculationHistory{
		Title:       workOrderInfo.Title,
		WorkOrder:   workOrderInfo.Id,
		State:       "结束工单",
		Circulation: "工单结束",
		Processor:   userInfo.NickName,
		ProcessorId: tools.GetUserId(c),
		Remarks:     "手动结束工单。",
		Status:      2,
	})

	tx.Commit()

	app.OK(c, nil, "工单已结束")
}

// 转交工单
func InversionWorkOrder(c *gin.Context) {
	var (
		cirHistoryValue   []process.CirculationHistory
		err               error
		workOrderInfo     process.WorkOrderInfo
		stateList         []map[string]interface{}
		stateValue        []byte
		currentState      map[string]interface{}
		userInfo          system.SysUser
		currentUserInfo   system.SysUser
		costDurationValue int64
		handle            service.Handle
		params            struct {
			WorkOrderId int    `json:"work_order_id"`
			NodeId      string `json:"node_id"`
			UserId      int    `json:"user_id"`
			Remarks     string `json:"remarks"`
		}
	)

	// 获取当前用户信息
	err = orm.Eloquent.Model(&currentUserInfo).
		Where("user_id = ?", tools.GetUserId(c)).
		Find(&currentUserInfo).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("当前用户查询失败，%v", err.Error()))
		return
	}

	err = c.ShouldBind(&params)
	if err != nil {
		app.Error(c, -1, err, "")
		return
	}

	// 查询工单信息
	err = orm.Eloquent.Model(&workOrderInfo).
		Where("id = ?", params.WorkOrderId).
		Find(&workOrderInfo).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("查询工单信息失败，%v", err.Error()))
		return
	}

	// 序列化节点数据
	err = json.Unmarshal(workOrderInfo.State, &stateList)
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("节点数据反序列化失败，%v", err.Error()))
		return
	}

	for _, s := range stateList {
		if s["id"].(string) == params.NodeId {
			s["processor"] = []interface{}{params.UserId}
			s["process_method"] = "person"
			currentState = s
			break
		}
	}

	stateValue, err = json.Marshal(stateList)
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("节点数据序列化失败，%v", err.Error()))
		return
	}

	tx := orm.Eloquent.Begin()

	// 更新数据
	err = tx.Model(&process.WorkOrderInfo{}).
		Where("id = ?", params.WorkOrderId).
		Update("state", stateValue).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("更新节点信息失败，%v", err.Error()))
		return
	}

	// 查询用户信息
	err = tx.Model(&system.SysUser{}).
		Where("user_id = ?", params.UserId).
		Find(&userInfo).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("查询用户信息失败，%v", err.Error()))
		return
	}

	// 流转历史写入
	err = orm.Eloquent.Model(&cirHistoryValue).
		Where("work_order = ?", params.WorkOrderId).
		Find(&cirHistoryValue).
		Order("create_time desc").Error
	if err != nil {
		tx.Rollback()
		return
	}
	for _, t := range cirHistoryValue {
		if t.Source != currentState["id"].(string) {
			costDuration := time.Since(t.CreatedAt.Time)
			costDurationValue = int64(costDuration) / 1000 / 1000 / 1000
		}
	}

	// 添加转交历史
	tx.Create(&process.CirculationHistory{
		Title:        workOrderInfo.Title,
		WorkOrder:    workOrderInfo.Id,
		State:        currentState["label"].(string),
		Circulation:  "转交工单",
		Processor:    currentUserInfo.NickName,
		ProcessorId:  tools.GetUserId(c),
		Remarks:      fmt.Sprintf("此阶段负责人已转交给《%v》", userInfo.NickName),
		Status:       2, // 其他
		CostDuration: costDurationValue,
	})

	tx.Commit()

	handle.SendEmail(workOrderInfo.Id)
	app.OK(c, nil, "工单已手动结单")
}

// 催办工单
func UrgeWorkOrder(c *gin.Context) {
	var (
		workOrderInfo  process.WorkOrderInfo
		sendToUserList []system.SysUser
		stateList      []interface{}
		userInfo       system.SysUser
	)
	workOrderId := c.DefaultQuery("workOrderId", "")
	if workOrderId == "" {
		app.Error(c, -1, errors.New("参数不正确，缺失workOrderId"), "")
		return
	}

	// 查询工单数据
	err := orm.Eloquent.Model(&process.WorkOrderInfo{}).Where("id = ?", workOrderId).Find(&workOrderInfo).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("查询工单信息失败，%v", err.Error()))
		return
	}

	// 确认是否可以催办
	if workOrderInfo.UrgeLastTime != 0 && (int(time.Now().Unix())-workOrderInfo.UrgeLastTime) < 600 {
		app.Error(c, -1, errors.New("十分钟内无法多次催办工单。"), "")
		return
	}

	// 获取当前工单处理人信息
	err = json.Unmarshal(workOrderInfo.State, &stateList)
	if err != nil {
		app.Error(c, -1, err, "")
		return
	}
	sendToUserList, err = service.GetPrincipalUserInfo(stateList, workOrderInfo.Creator)

	// 查询创建人信息
	err = orm.Eloquent.Model(&system.SysUser{}).Where("user_id = ?", workOrderInfo.Creator).Find(&userInfo).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("创建人信息查询失败，%v", err.Error()))
		return
	}

	// 发送催办提醒
	bodyData := notify.BodyData{
		SendTo: map[string]interface{}{
			"userList": sendToUserList,
		},
		Subject:     "您被催办工单了，请及时处理。",
		Description: "您有一条待办工单，请及时处理，工单描述如下",
		Classify:    []int{1}, // todo 1 表示邮箱，后续添加了其他的在重新补充
		ProcessId:   workOrderInfo.Process,
		Id:          workOrderInfo.Id,
		Title:       workOrderInfo.Title,
		Creator:     userInfo.NickName,
		Priority:    workOrderInfo.Priority,
		CreatedAt:   workOrderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	err = bodyData.SendNotify()
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("催办提醒发送失败，%v", err.Error()))
		return
	}

	// 更新数据库
	err = orm.Eloquent.Model(&process.WorkOrderInfo{}).Where("id = ?", workOrderInfo.Id).Updates(map[string]interface{}{
		"urge_count":     workOrderInfo.UrgeCount + 1,
		"urge_last_time": int(time.Now().Unix()),
	}).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("更新催办信息失败，%v", err.Error()))
		return
	}

	app.OK(c, "", "")
}

// 主动处理
func ActiveOrder(c *gin.Context) {
	var (
		workOrderId string
		err         error
		stateValue  []struct {
			ID            string `json:"id"`
			Label         string `json:"label"`
			ProcessMethod string `json:"process_method"`
			Processor     []int  `json:"processor"`
		}
		stateValueByte []byte
	)

	workOrderId = c.Param("id")

	err = c.ShouldBind(&stateValue)
	if err != nil {
		app.Error(c, -1, err, "")
		return
	}

	stateValueByte, err = json.Marshal(stateValue)
	if err != nil {
		app.Error(c, -1, fmt.Errorf("转byte失败，%v", err.Error()), "")
		return
	}

	err = orm.Eloquent.Model(&process.WorkOrderInfo{}).
		Where("id = ?", workOrderId).
		Update("state", stateValueByte).Error
	if err != nil {
		app.Error(c, -1, fmt.Errorf("接单失败，%v", err.Error()), "")
		return
	}

	app.OK(c, "", "接单成功，请及时处理")
}

// 删除工单
func DeleteWorkOrder(c *gin.Context) {

	workOrderId := c.Param("id")

	err := orm.Eloquent.Delete(&process.WorkOrderInfo{}, workOrderId).Error
	if err != nil {
		app.Error(c, -1, err, "")
		return
	}

	app.OK(c, "", "工单已删除")
}

// 重开工单
func ReopenWorkOrder(c *gin.Context) {
	var (
		err           error
		id            string
		workOrder     process.WorkOrderInfo
		processInfo   process.Info
		structure     map[string]interface{}
		startId       string
		label         string
		jsonState     []byte
		relatedPerson []byte
		newWorkOrder  process.WorkOrderInfo
		workOrderData []*process.TplData
	)

	id = c.Param("id")

	// 查询当前ID的工单信息
	err = orm.Eloquent.Find(&workOrder, id).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("查询工单信息失败, %s", err.Error()))
		return
	}

	// 创建新的工单
	err = orm.Eloquent.Find(&processInfo, workOrder.Process).Error
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("查询流程信息失败, %s", err.Error()))
		return
	}
	err = json.Unmarshal(processInfo.Structure, &structure)
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("Json序列化失败, %s", err.Error()))
		return
	}
	for _, node := range structure["nodes"].([]interface{}) {
		if node.(map[string]interface{})["clazz"] == "start" {
			startId = node.(map[string]interface{})["id"].(string)
			label = node.(map[string]interface{})["label"].(string)
		}
	}

	state := []map[string]interface{}{
		{"id": startId, "label": label, "processor": []int{tools.GetUserId(c)}, "process_method": "person"},
	}
	jsonState, err = json.Marshal(state)
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("Json序列化失败, %s", err.Error()))
		return
	}

	relatedPerson, err = json.Marshal([]int{tools.GetUserId(c)})
	if err != nil {
		app.Error(c, -1, err, fmt.Sprintf("Json序列化失败, %s", err.Error()))
		return
	}

	tx := orm.Eloquent.Begin()

	newWorkOrder = process.WorkOrderInfo{
		Title:         workOrder.Title,
		Priority:      workOrder.Priority,
		Process:       workOrder.Process,
		Classify:      workOrder.Classify,
		State:         jsonState,
		RelatedPerson: relatedPerson,
		Creator:       tools.GetUserId(c),
	}
	err = tx.Create(&newWorkOrder).Error
	if err != nil {
		tx.Rollback()
		app.Error(c, -1, err, fmt.Sprintf("新建工单失败, %s", err.Error()))
		return
	}

	// 查询工单数据
	err = orm.Eloquent.Model(&process.TplData{}).Where("work_order = ?", id).Find(&workOrderData).Error
	if err != nil {
		tx.Rollback()
		app.Error(c, -1, err, fmt.Sprintf("查询工单数据失败, %s", err.Error()))
		return
	}

	for _, d := range workOrderData {
		d.WorkOrder = newWorkOrder.Id
		d.Id = 0
		err = tx.Create(d).Error
		if err != nil {
			tx.Rollback()
			app.Error(c, -1, err, fmt.Sprintf("创建工单数据失败, %s", err.Error()))
			return
		}
	}

	tx.Commit()

	app.OK(c, nil, "")
}

func GetprojectList(c *gin.Context) {
	var (
		result interface{}
		err    error
		params pagination.ListRequest
	)

	// 绑定分页参数
	if err = c.ShouldBindQuery(&params); err != nil {
		app.Error(c, -1, err, "参数绑定失败")
		return
	}

	// 创建连表查询
	db := orm.Eloquent.Model(&process.TplData{}).
		Select(`
		p_work_order_tpl_data.id as tpl_id,
		p_work_order_info.id as work_order_id,
		p_work_order_tpl_data.create_time,
		p_work_order_tpl_data.update_time,
		p_work_order_tpl_data.work_order,
		p_work_order_tpl_data.form_structure,
        p_work_order_tpl_data.form_data,
		p_work_order_info.title,
		p_work_order_info.priority,
		p_work_order_info.create_time,
		p_work_order_info.process,
		p_work_order_info.state,
		p_work_order_info.is_end,
		p_work_order_info.related_person,
		p_work_order_info.creator,
		p_work_order_info.classify
	`).
		Joins("LEFT JOIN p_work_order_info ON p_work_order_tpl_data.work_order = p_work_order_info.id").
		Where("JSON_EXTRACT(p_work_order_tpl_data.form_structure, '$.id') IN (7)")

	// 时间范围筛选（使用工单表的创建时间）
	if startTime := c.Query("startTime"); startTime != "" {
		db = db.Where("p_work_order_info.create_time >= ?", startTime)
	}
	if endTime := c.Query("endTime"); endTime != "" {
		db = db.Where("p_work_order_info.create_time <= ?", endTime)
	}
	if isEnd := c.Query("isEnd"); isEnd != "" {
		db = db.Where("p_work_order_info.is_end = ?", isEnd)
	}
	if creator := c.Query("creator"); creator != "" {
		db = db.Where("p_work_order_info.creator = ?", creator)
	}
	if title := c.Query("title"); title != "" {
		db = db.Where("p_work_order_info.title LIKE CONCAT('%',?,'%')", title)
	}
	if priority := c.Query("priority"); priority != "" {
		priorityID, err := strconv.Atoi(priority)
		if err != nil {
			app.Error(c, -1, err, "处理器ID格式错误")
			return
		}
		db = db.Where("JSON_EXTRACT(p_work_order_tpl_data.form_data, '$.rate_1741574352000_36134') in (?)", priorityID)
	}
	if processor := c.Query("processor"); processor != "" {
		processorID, err := strconv.Atoi(processor)
		if err != nil {
			app.Error(c, -1, err, "处理器ID格式错误")
			return
		}
		db = db.Where(`JSON_CONTAINS(p_work_order_info.state->'$[*].processor', JSON_ARRAY(?))`, processorID)
	}

	// 分页查询
	result, err = pagination.Paging(&pagination.Param{
		C:       c,
		DB:      db,
		ShowSQL: true,
		OrderBy: "p_work_order_info.id DESC",
	}, &[]ProjectListResult{}, map[string]map[string]interface{}{})

	if err != nil {
		app.Error(c, -1, err, "查询失败")
		return
	}

	app.OK(c, result, "")
}

func DecryptPhone(c *gin.Context) {
	type params struct {
		Code          string `json:"code"`
		EncryptedData string `json:"encryptedData"`
		IV            string `json:"iv"`
	}

	var p params
	if err := c.ShouldBindJSON(&p); err != nil {
		app.Error(c, -1, err, "参数绑定失败")
		return
	}

	// 1. Get access token
	// accessToken, err := getWeChatAccessToken()
	accessToken, err := getCachedAccessToken()
	fmt.Println("accessToken ", string(accessToken))
	if err != nil {
		app.Error(c, 1001, err, "获取access_token失败")
		return
	}

	// 2. Call WeChat phone number API
	phoneInfo, err := getWeChatPhoneNumber(accessToken, p.Code)
	if err != nil {
		app.Error(c, 1002, err, "获取手机号失败")
		return
	}
	fmt.Println("phoneInfo:", phoneInfo)
	// 3. Return phone number
	app.OK(c, gin.H{
		"phone": phoneInfo.PhoneNumber,
	}, "成功获取手机号")
}

// func getWeChatAccessToken() (string, error) {
// 	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
// 		"wx94f204acca2ce859",
// 		"f92c275c583d02c8b134a2a501bcc92c")

// 	resp, err := http.Get(url)
// 	if err != nil {
// 		return "", err
// 	}
// 	defer resp.Body.Close()

// 	var result struct {
// 		AccessToken string `json:"access_token"`
// 		ErrCode     int    `json:"errcode"`
// 		ErrMsg      string `json:"errmsg"`
// 	}
// 	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
// 		return "", err
// 	}

// 	if result.ErrCode != 0 {
// 		return "", fmt.Errorf("微信接口错误: %d-%s", result.ErrCode, result.ErrMsg)
// 	}

// 	return result.AccessToken, nil
// }

func getWeChatPhoneNumber(accessToken, code string) (*struct {
	PhoneNumber string `json:"phoneNumber"`
}, error) {
	url := fmt.Sprintf("https://api.weixin.qq.com/wxa/business/getuserphonenumber?access_token=%s", accessToken)

	body, _ := json.Marshal(map[string]string{"code": code})
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		PhoneInfo struct {
			PhoneNumber string `json:"phoneNumber"`
		} `json:"phone_info"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.ErrCode != 0 {
		return nil, fmt.Errorf("微信接口错误: %d-%s", result.ErrCode, result.ErrMsg)
	}

	return &struct {
		PhoneNumber string `json:"phoneNumber"`
	}{
		PhoneNumber: result.PhoneInfo.PhoneNumber,
	}, nil
}

func fetchNewAccessToken() (string, int64, error) {
	url := fmt.Sprintf("https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=%s&secret=%s",
		"{appid}", "{secret}")

	resp, err := http.Get(url)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", 0, err
	}

	if result.ErrCode != 0 {
		return "", 0, fmt.Errorf("微信接口错误: %d-%s", result.ErrCode, result.ErrMsg)
	}

	return result.AccessToken, result.ExpiresIn, nil
}

// 获取带缓存的access_token
func getCachedAccessToken() (string, error) {
	// 先尝试读取缓存
	wechatTokenCache.RLock()
	if time.Now().Unix() < wechatTokenCache.ExpiresAt-300 { // 提前5分钟刷新
		defer wechatTokenCache.RUnlock()
		return wechatTokenCache.AccessToken, nil
	}
	wechatTokenCache.RUnlock()

	// 加锁防止并发刷新
	wechatTokenCache.Lock()
	defer wechatTokenCache.Unlock()

	// 双重检查锁
	if time.Now().Unix() < wechatTokenCache.ExpiresAt-300 {
		return wechatTokenCache.AccessToken, nil
	}

	// 重新获取token
	token, expiresIn, err := fetchNewAccessToken()
	if err != nil {
		return "", err
	}

	// 更新缓存（微信返回的expires_in通常是7200秒）
	wechatTokenCache.AccessToken = token
	wechatTokenCache.ExpiresAt = time.Now().Unix() + expiresIn

	return token, nil
}
