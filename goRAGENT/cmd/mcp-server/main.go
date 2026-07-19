package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ===================== MCP JSON-RPC 2.0 Protocol =====================

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      int             `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      int             `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ===================== MCP 数据类型 =====================

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callToolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError"`
}

// ===================== Tool Registry =====================

var tools = map[string]toolDef{}
var handlers = map[string]func(args map[string]any) callToolResult{}

func register(t toolDef, h func(args map[string]any) callToolResult) {
	tools[t.Name] = t
	handlers[t.Name] = h
}

// ===================== MCP HTTP Handler =====================

func mcpHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", 405)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	defer r.Body.Close()

	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}

	var respData json.RawMessage
	var rpcErr *rpcError

	switch req.Method {
	case "initialize":
		respData, rpcErr = handleInitialize(req.Params)
	case "notifications/initialized":
		respData, _ = json.Marshal(map[string]string{})
	case "tools/list":
		respData, rpcErr = handleListTools()
	case "tools/call":
		respData, rpcErr = handleCallTool(req.Params)
	default:
		rpcErr = &rpcError{Code: -32601, Message: "Method not found: " + req.Method}
	}

	resp := rpcResponse{
		JSONRPC: "2.0",
		Result:  respData,
		Error:   rpcErr,
		ID:      req.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleInitialize(params json.RawMessage) (json.RawMessage, *rpcError) {
	result := map[string]any{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]string{
			"name":    "ragent-mcp-server-go",
			"version": "0.0.1",
		},
		"capabilities": map[string]any{},
	}
	b, _ := json.Marshal(result)
	return b, nil
}

func handleListTools() (json.RawMessage, *rpcError) {
	toolList := make([]toolDef, 0, len(tools))
	for _, t := range tools {
		toolList = append(toolList, t)
	}
	result := map[string]any{"tools": toolList}
	b, _ := json.Marshal(result)
	return b, nil
}

func handleCallTool(params json.RawMessage) (json.RawMessage, *rpcError) {
	var p callToolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}

	h, ok := handlers[p.Name]
	if !ok {
		return nil, &rpcError{Code: -32602, Message: "Unknown tool: " + p.Name}
	}

	if p.Arguments == nil {
		p.Arguments = map[string]any{}
	}

	result := h(p.Arguments)
	b, _ := json.Marshal(result)
	return b, nil
}

// ====================== Tool: sales_query ======================

var salesRegions = []string{"华东", "华南", "华北", "西南", "西北"}
var salesProducts = []string{"企业版", "专业版", "基础版"}
var salesByRegion = map[string][]string{
	"华东": {"张三", "李四", "王五"},
	"华南": {"赵六", "钱七", "孙八"},
	"华北": {"周九", "吴十", "郑冬"},
	"西南": {"陈春", "林夏", "黄秋"},
	"西北": {"刘一", "杨二", "马三"},
}
var customerPool = []string{
	"腾讯科技", "阿里巴巴", "字节跳动", "美团点评", "京东集团",
	"百度在线", "网易公司", "小米科技", "华为技术", "中兴通讯",
	"用友网络", "金蝶软件", "浪潮集团", "东软集团", "科大讯飞",
	"三一重工", "中联重科", "格力电器", "美的集团", "海尔智家",
}

type salesRecord struct {
	Region      string
	SalesPerson string
	Product     string
	Customer    string
	Amount      float64
	Date        string
}

func generateSalesData(period string) []salesRecord {
	now := time.Now()
	var start, end time.Time
	switch period {
	case "上月":
		start = time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.Local)
		end = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).Add(-time.Second)
	case "本季度":
		q := (int(now.Month()) - 1) / 3
		start = time.Date(now.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, time.Local)
		end = now
	case "上季度":
		q := (int(now.Month()) - 1) / 3
		end = time.Date(now.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, time.Local).Add(-time.Second)
		start = time.Date(end.Year(), time.Month(((q-1+4)%4)*3+1), 1, 0, 0, 0, 0, time.Local)
	case "本年":
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local)
		end = now
	default: // 本月
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		end = now
	}

	rng := rand.New(rand.NewSource(start.Unix()))
	records := make([]salesRecord, 0)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		ordersPerDay := 3 + rng.Intn(6)
		for i := 0; i < ordersPerDay; i++ {
			region := salesRegions[rng.Intn(len(salesRegions))]
			product := salesProducts[rng.Intn(len(salesProducts))]
			var amount float64
			switch product {
			case "企业版":
				amount = 50 + rng.Float64()*150
			case "专业版":
				amount = 10 + rng.Float64()*40
			default:
				amount = 1 + rng.Float64()*9
			}
			amount = math.Round(amount*100) / 100
			records = append(records, salesRecord{
				Region:      region,
				SalesPerson: salesByRegion[region][rng.Intn(3)],
				Product:     product,
				Customer:    customerPool[rng.Intn(len(customerPool))] + fmt.Sprintf("%d", d.Day()),
				Amount:      amount,
				Date:        d.Format("2006-01-02"),
			})
		}
	}
	return records
}

func filterSales(data []salesRecord, region, product, salesPerson string) []salesRecord {
	var filtered []salesRecord
	for _, r := range data {
		if region != "" && r.Region != region {
			continue
		}
		if product != "" && r.Product != product {
			continue
		}
		if salesPerson != "" && r.SalesPerson != salesPerson {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

func buildSalesSummary(data []salesRecord, region, product, salesPerson, period string) string {
	var totalAmount float64
	byProduct := map[string]float64{}
	byRegion := map[string]float64{}
	for _, r := range data {
		totalAmount += r.Amount
		byProduct[r.Product] += r.Amount
		byRegion[r.Region] += r.Amount
	}
	orderCount := len(data)
	avgAmount := 0.0
	if orderCount > 0 {
		avgAmount = totalAmount / float64(orderCount)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s 销售数据汇总】\n\n", period))
	var filters []string
	if region != "" {
		filters = append(filters, "地区: "+region)
	}
	if product != "" {
		filters = append(filters, "产品: "+product)
	}
	if salesPerson != "" {
		filters = append(filters, "销售: "+salesPerson)
	}
	if len(filters) > 0 {
		sb.WriteString("筛选条件: " + strings.Join(filters, "，") + "\n\n")
	}
	sb.WriteString(fmt.Sprintf("总销售额: ¥%.2f 万\n", totalAmount))
	sb.WriteString(fmt.Sprintf("成交订单: %d 笔\n", orderCount))
	sb.WriteString(fmt.Sprintf("平均单价: ¥%.2f 万\n", avgAmount))

	if product == "" && len(byProduct) > 0 {
		sb.WriteString("\n【按产品分布】\n")
		type kv struct{ k string; v float64 }
		var list []kv
		for k, v := range byProduct { list = append(list, kv{k, v}) }
		sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
		for _, e := range list {
			sb.WriteString(fmt.Sprintf("  %s: ¥%.2f 万 (%.1f%%)\n", e.k, e.v, e.v/totalAmount*100))
		}
	}
	if region == "" && len(byRegion) > 0 {
		sb.WriteString("\n【按地区分布】\n")
		type kv struct{ k string; v float64 }
		var list []kv
		for k, v := range byRegion { list = append(list, kv{k, v}) }
		sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
		for _, e := range list {
			sb.WriteString(fmt.Sprintf("  %s: ¥%.2f 万 (%.1f%%)\n", e.k, e.v, e.v/totalAmount*100))
		}
	}
	return strings.TrimSpace(sb.String())
}

func buildSalesRanking(data []salesRecord, region, period string, limit int) string {
	bySales := map[string]float64{}
	for _, r := range data {
		bySales[r.SalesPerson] += r.Amount
	}
	type kv struct{ k string; v float64 }
	var list []kv
	for k, v := range bySales { list = append(list, kv{k, v}) }
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	if len(list) > limit {
		list = list[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s", period))
	if region != "" {
		sb.WriteString(" " + region)
	}
	sb.WriteString(" 销售排名】\n\n")
	if len(list) == 0 {
		sb.WriteString("暂无销售数据")
	} else {
		for i, e := range list {
			sb.WriteString(fmt.Sprintf("第%d名: %s - ¥%.2f 万\n", i+1, e.k, e.v))
		}
	}
	return strings.TrimSpace(sb.String())
}

func buildSalesDetail(data []salesRecord, region, period string, limit int) string {
	sort.Slice(data, func(i, j int) bool { return data[i].Amount > data[j].Amount })
	if len(data) > limit {
		data = data[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s", period))
	if region != "" {
		sb.WriteString(" " + region)
	}
	sb.WriteString(fmt.Sprintf(" 销售明细】\n\n共 %d 条记录：\n\n", len(data)))
	for i, r := range data {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Customer))
		sb.WriteString(fmt.Sprintf("   产品: %s | 金额: ¥%.2f 万\n", r.Product, r.Amount))
		sb.WriteString(fmt.Sprintf("   销售: %s | 地区: %s | 日期: %s\n\n", r.SalesPerson, r.Region, r.Date))
	}
	return strings.TrimSpace(sb.String())
}

func buildSalesTrend(data []salesRecord, region, period string) string {
	byWeek := map[string]float64{}
	for _, r := range data {
		d, _ := time.Parse("2006-01-02", r.Date)
		week := fmt.Sprintf("第%d周", (d.Day()-1)/7+1)
		byWeek[week] += r.Amount
	}
	type kv struct{ k string; v float64 }
	var list []kv
	for k, v := range byWeek { list = append(list, kv{k, v}) }
	sort.Slice(list, func(i, j int) bool { return list[i].k < list[j].k })

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s", period))
	if region != "" {
		sb.WriteString(" " + region)
	}
	sb.WriteString(" 销售趋势】\n\n")
	for _, e := range list {
		sb.WriteString(fmt.Sprintf("%s: ¥%.2f 万\n", e.k, e.v))
	}
	return strings.TrimSpace(sb.String())
}

func handleSalesQuery(args map[string]any) callToolResult {
	region := strArg(args, "region")
	period := strArg(args, "period")
	product := strArg(args, "product")
	salesPerson := strArg(args, "salesPerson")
	queryType := strArg(args, "queryType")
	limit := intArg(args, "limit")

	if period == "" {
		period = "本月"
	}
	if queryType == "" {
		queryType = "summary"
	}
	if limit <= 0 {
		limit = 10
	}

	data := generateSalesData(period)
	filtered := filterSales(data, region, product, salesPerson)

	var result string
	switch queryType {
	case "ranking":
		result = buildSalesRanking(filtered, region, period, limit)
	case "detail":
		result = buildSalesDetail(filtered, region, period, limit)
	case "trend":
		result = buildSalesTrend(filtered, region, period)
	default:
		result = buildSalesSummary(filtered, region, product, salesPerson, period)
	}

	log.Printf("[sales_query] queryType=%s region=%s period=%s records=%d", queryType, region, period, len(filtered))
	return callToolResult{
		Content: []textContent{{Type: "text", Text: result}},
		IsError: false,
	}
}

// ====================== Tool: ticket_query ======================

var ticketRegions = []string{"华东", "华南", "华北", "西南", "西北"}
var ticketProducts = []string{"企业版", "专业版", "基础版"}
var ticketStatuses = []string{"待处理", "处理中", "已解决", "已关闭"}
var ticketPriorities = []string{"紧急", "高", "中", "低"}
var ticketCategories = []string{"功能异常", "性能问题", "安装部署", "使用咨询", "数据问题", "权限问题"}
var customersByRegion = map[string][]string{
	"华东": {"腾讯科技", "阿里巴巴", "字节跳动", "网易公司"},
	"华南": {"美团点评", "京东集团", "小米科技", "格力电器"},
	"华北": {"百度在线", "华为技术", "中兴通讯", "用友网络"},
	"西南": {"科大讯飞", "金蝶软件", "三一重工", "中联重科"},
	"西北": {"浪潮集团", "东软集团", "美的集团", "海尔智家"},
}
var engineersByRegion = map[string][]string{
	"华东": {"工程师A1", "工程师A2"},
	"华南": {"工程师B1", "工程师B2"},
	"华北": {"工程师C1", "工程师C2"},
	"西南": {"工程师D1", "工程师D2"},
	"西北": {"工程师E1", "工程师E2"},
}
var issueTemplates = []string{
	"系统登录后页面白屏无法操作",
	"报表导出功能超时失败",
	"用户权限配置不生效",
	"数据同步延迟超过预期",
	"批量导入数据格式校验异常",
	"API接口调用返回500错误",
	"定时任务未按计划执行",
	"搜索功能结果不准确",
	"通知消息无法正常推送",
	"文件上传大小限制配置无效",
	"仪表盘数据展示不一致",
	"多租户数据隔离存在问题",
	"审批流程节点卡住无法流转",
	"移动端页面适配显示异常",
	"数据备份任务执行失败",
}

type ticketRecord struct {
	TicketID   string
	Region     string
	Customer   string
	Product    string
	Title      string
	Category   string
	Priority   string
	Status     string
	Engineer   string
	CreateDate string
}

func generateTicketData() []ticketRecord {
	now := time.Now()
	rng := rand.New(rand.NewSource(now.Unix()))
	ticketSeq := 1
	var records []ticketRecord

	for d := 0; d < 30; d++ {
		date := now.AddDate(0, 0, -d)
		if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
			continue
		}
		ticketsPerDay := 2 + rng.Intn(5)
		for i := 0; i < ticketsPerDay; i++ {
			t := ticketRecord{}
			t.TicketID = fmt.Sprintf("TK-%s-%04d", now.Format("200601"), ticketSeq)
			ticketSeq++
			t.Region = ticketRegions[rng.Intn(len(ticketRegions))]
			t.Customer = customersByRegion[t.Region][rng.Intn(4)]
			t.Product = ticketProducts[rng.Intn(len(ticketProducts))]
			t.Title = issueTemplates[rng.Intn(len(issueTemplates))]
			t.Category = ticketCategories[rng.Intn(len(ticketCategories))]
			t.Engineer = engineersByRegion[t.Region][rng.Intn(2)]
			t.CreateDate = date.Format("2006-01-02")

			pw := rng.Intn(100)
			switch {
			case pw < 5:
				t.Priority = "紧急"
			case pw < 20:
				t.Priority = "高"
			case pw < 60:
				t.Priority = "中"
			default:
				t.Priority = "低"
			}

			if d > 7 {
				if rng.Intn(100) < 80 {
					t.Status = "已关闭"
				} else {
					t.Status = "已解决"
				}
			} else if d > 3 {
				sw := rng.Intn(100)
				switch {
				case sw < 30:
					t.Status = "已解决"
				case sw < 60:
					t.Status = "已关闭"
				case sw < 85:
					t.Status = "处理中"
				default:
					t.Status = "待处理"
				}
			} else {
				sw := rng.Intn(100)
				switch {
				case sw < 35:
					t.Status = "待处理"
				case sw < 70:
					t.Status = "处理中"
				case sw < 90:
					t.Status = "已解决"
				default:
					t.Status = "已关闭"
				}
			}
			records = append(records, t)
		}
	}
	return records
}

func filterTickets(data []ticketRecord, region, status, priority, product, customerName string) []ticketRecord {
	var filtered []ticketRecord
	for _, t := range data {
		if region != "" && t.Region != region {
			continue
		}
		if status != "" && t.Status != status {
			continue
		}
		if priority != "" && t.Priority != priority {
			continue
		}
		if product != "" && t.Product != product {
			continue
		}
		if customerName != "" && !strings.Contains(t.Customer, customerName) {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

func buildTicketSummary(data []ticketRecord, region, status, priority, product string) string {
	var pending, inProgress, resolved, closed int64
	var urgent, high int64
	for _, t := range data {
		switch t.Status {
		case "待处理":
			pending++
		case "处理中":
			inProgress++
		case "已解决":
			resolved++
		case "已关闭":
			closed++
		}
		if t.Priority == "紧急" {
			urgent++
		}
		if t.Priority == "高" {
			high++
		}
	}
	total := len(data)

	var sb strings.Builder
	sb.WriteString("【客户工单汇总概览】\n\n")
	var filters []string
	if region != "" {
		filters = append(filters, "地区: "+region)
	}
	if status != "" {
		filters = append(filters, "状态: "+status)
	}
	if priority != "" {
		filters = append(filters, "优先级: "+priority)
	}
	if product != "" {
		filters = append(filters, "产品: "+product)
	}
	if len(filters) > 0 {
		sb.WriteString("筛选条件: " + strings.Join(filters, "，") + "\n\n")
	}
	sb.WriteString(fmt.Sprintf("工单总数: %d 个\n\n", total))
	sb.WriteString("【状态分布】\n")
	sb.WriteString(fmt.Sprintf("  待处理: %d 个\n", pending))
	sb.WriteString(fmt.Sprintf("  处理中: %d 个\n", inProgress))
	sb.WriteString(fmt.Sprintf("  已解决: %d 个\n", resolved))
	sb.WriteString(fmt.Sprintf("  已关闭: %d 个\n\n", closed))
	if total > 0 {
		sb.WriteString(fmt.Sprintf("解决率: %.1f%%\n", float64(resolved+closed)*100/float64(total)))
	}
	if urgent+high > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠ 紧急/高优先级工单: %d 个（紧急 %d，高 %d）\n", urgent+high, urgent, high))
	}

	byProduct := map[string]int64{}
	for _, t := range data {
		byProduct[t.Product]++
	}
	if product == "" && len(byProduct) > 0 {
		sb.WriteString("\n【按产品分布】\n")
		type kv struct{ k string; v int64 }
		var list []kv
		for k, v := range byProduct { list = append(list, kv{k, v}) }
		sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
		for _, e := range list {
			sb.WriteString(fmt.Sprintf("  %s: %d 个\n", e.k, e.v))
		}
	}

	byRegion := map[string]int64{}
	for _, t := range data {
		byRegion[t.Region]++
	}
	if region == "" && len(byRegion) > 0 {
		sb.WriteString("\n【按地区分布】\n")
		type kv struct{ k string; v int64 }
		var list []kv
		for k, v := range byRegion { list = append(list, kv{k, v}) }
		sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
		for _, e := range list {
			sb.WriteString(fmt.Sprintf("  %s: %d 个\n", e.k, e.v))
		}
	}
	return strings.TrimSpace(sb.String())
}

func buildTicketList(data []ticketRecord, limit int) string {
	prioOrder := map[string]int{"紧急": 0, "高": 1, "中": 2, "低": 3}
	sort.Slice(data, func(i, j int) bool {
		if prioOrder[data[i].Priority] != prioOrder[data[j].Priority] {
			return prioOrder[data[i].Priority] < prioOrder[data[j].Priority]
		}
		return data[i].CreateDate > data[j].CreateDate
	})
	if len(data) > limit {
		data = data[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【工单列表】共 %d 条（按优先级排序）\n\n", len(data)))
	for i, t := range data {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, t.TicketID, t.Title))
		sb.WriteString(fmt.Sprintf("   客户: %s | 产品: %s | 地区: %s\n", t.Customer, t.Product, t.Region))
		sb.WriteString(fmt.Sprintf("   优先级: %s | 状态: %s | 分类: %s\n", t.Priority, t.Status, t.Category))
		sb.WriteString(fmt.Sprintf("   处理人: %s | 创建时间: %s\n\n", t.Engineer, t.CreateDate))
	}
	return strings.TrimSpace(sb.String())
}

func buildTicketStats(data []ticketRecord) string {
	if len(data) == 0 {
		return "暂无工单数据"
	}
	var sb strings.Builder
	sb.WriteString("【工单统计分析】\n\n")

	byCategory := map[string]int64{}
	for _, t := range data {
		byCategory[t.Category]++
	}
	sb.WriteString("【问题分类统计】\n")
	type kvi struct{ k string; v int64 }
	var clist []kvi
	for k, v := range byCategory { clist = append(clist, kvi{k, v}) }
	sort.Slice(clist, func(i, j int) bool { return clist[i].v > clist[j].v })
	for _, e := range clist {
		sb.WriteString(fmt.Sprintf("  %s: %d 个 (%.1f%%)\n", e.k, e.v, float64(e.v)*100/float64(len(data))))
	}

	sb.WriteString("\n【处理人工单量排名】\n")
	byEngineer := map[string]int64{}
	for _, t := range data {
		if t.Status == "待处理" || t.Status == "处理中" {
			byEngineer[t.Engineer]++
		}
	}
	type kvi2 struct{ k string; v int64 }
	var elist []kvi2
	for k, v := range byEngineer { elist = append(elist, kvi2{k, v}) }
	sort.Slice(elist, func(i, j int) bool { return elist[i].v > elist[j].v })
	limit := 5
	if len(elist) < limit {
		limit = len(elist)
	}
	for i := 0; i < limit; i++ {
		sb.WriteString(fmt.Sprintf("  %s: %d 个待处理\n", elist[i].k, elist[i].v))
	}
	return strings.TrimSpace(sb.String())
}

func handleTicketQuery(args map[string]any) callToolResult {
	region := strArg(args, "region")
	status := strArg(args, "status")
	priority := strArg(args, "priority")
	product := strArg(args, "product")
	customerName := strArg(args, "customerName")
	queryType := strArg(args, "queryType")
	limit := intArg(args, "limit")

	if queryType == "" {
		queryType = "summary"
	}
	if limit <= 0 {
		limit = 10
	}

	data := generateTicketData()
	filtered := filterTickets(data, region, status, priority, product, customerName)

	var result string
	switch queryType {
	case "list":
		result = buildTicketList(filtered, limit)
	case "stats":
		result = buildTicketStats(filtered)
	default:
		result = buildTicketSummary(filtered, region, status, priority, product)
	}

	log.Printf("[ticket_query] queryType=%s region=%s status=%s records=%d", queryType, region, status, len(filtered))
	return callToolResult{
		Content: []textContent{{Type: "text", Text: result}},
		IsError: false,
	}
}

// ====================== Tool: weather_query ======================

var cityCoords = map[string][2]float64{
	"北京": {39.9, 116.4}, "上海": {31.2, 121.5}, "广州": {23.1, 113.3},
	"深圳": {22.5, 114.1}, "杭州": {30.3, 120.2}, "成都": {30.6, 104.1},
	"武汉": {30.6, 114.3}, "南京": {32.1, 118.8}, "西安": {34.3, 108.9},
	"重庆": {29.6, 106.5}, "长沙": {28.2, 112.9}, "天津": {39.1, 117.2},
	"苏州": {31.3, 120.6}, "郑州": {34.7, 113.6}, "青岛": {36.1, 120.4},
	"大连": {38.9, 121.6}, "厦门": {24.5, 118.1}, "昆明": {25.0, 102.7},
	"哈尔滨": {45.8, 126.5}, "三亚": {18.3, 109.5},
}

var weatherTypesSpring = []string{"晴", "多云", "阴", "小雨", "阵雨", "多云转晴"}
var weatherTypesSummer = []string{"晴", "多云", "雷阵雨", "大雨", "暴雨", "多云转阴"}
var weatherTypesAutumn = []string{"晴", "多云", "阴", "小雨", "晴转多云", "多云转晴"}
var weatherTypesWinter = []string{"晴", "多云", "阴", "小雪", "中雪", "晴转多云", "雾"}
var windDirections = []string{"东风", "南风", "西风", "北风", "东南风", "西北风", "东北风", "西南风"}

type weatherData struct {
	WeatherType   string
	CurrentTemp   int
	HighTemp      int
	LowTemp       int
	Humidity      int
	WindDirection string
	WindLevel     string
	AirQuality    string
}

func generateWeather(city string, date time.Time) weatherData {
	coords := cityCoords[city]
	lat := coords[0]
	month := int(date.Month())
	season := 0 // spring
	switch {
	case month >= 6 && month <= 8:
		season = 1 // summer
	case month >= 9 && month <= 11:
		season = 2 // autumn
	case month == 12 || month <= 2:
		season = 3 // winter
	}

	seed := date.Unix()/86400*31 + int64(len(city)+int(city[0]))
	rng := rand.New(rand.NewSource(seed))

	baseTemp := 15.0 - (lat-25)*0.5
	switch season {
	case 1:
		baseTemp = 30 - (lat-25)*0.3
	case 2:
		baseTemp = 18 - (lat-25)*0.5
	case 3:
		baseTemp = 5 - (lat-25)*0.8
	}

	highTemp := int(baseTemp) + 3 + rng.Intn(6)
	lowTemp := int(baseTemp) - 3 - rng.Intn(5)
	currentTemp := lowTemp + rng.Intn(max(1, highTemp-lowTemp))

	var weatherTypes []string
	switch season {
	case 0:
		weatherTypes = weatherTypesSpring
	case 1:
		weatherTypes = weatherTypesSummer
	case 2:
		weatherTypes = weatherTypesAutumn
	default:
		weatherTypes = weatherTypesWinter
	}
	weatherType := weatherTypes[rng.Intn(len(weatherTypes))]

	humidity := 40 + rng.Intn(30)
	switch season {
	case 1:
		humidity = 60 + rng.Intn(30)
	case 3:
		humidity = 20 + rng.Intn(30)
	}
	if strings.Contains(weatherType, "雨") || strings.Contains(weatherType, "雪") {
		humidity = min(95, humidity+20)
	}

	windDirection := windDirections[rng.Intn(len(windDirections))]
	windForce := 1 + rng.Intn(5)
	windLevel := fmt.Sprintf("%d-%d级", windForce, windForce+1)

	aqiBase := 30 + rng.Intn(120)
	if lat > 35 {
		aqiBase += 20
	}
	var airQuality string
	switch {
	case aqiBase <= 50:
		airQuality = "优"
	case aqiBase <= 100:
		airQuality = "良"
	case aqiBase <= 150:
		airQuality = "轻度污染"
	default:
		airQuality = "中度污染"
	}

	return weatherData{
		WeatherType: weatherType, CurrentTemp: currentTemp,
		HighTemp: highTemp, LowTemp: lowTemp, Humidity: humidity,
		WindDirection: windDirection, WindLevel: windLevel, AirQuality: airQuality,
	}
}

func handleWeatherQuery(args map[string]any) callToolResult {
	city := strArg(args, "city")
	queryType := strArg(args, "queryType")
	days := intArg(args, "days")

	if city == "" {
		return callToolResult{
			Content: []textContent{{Type: "text", Text: "请提供城市名称，当前支持：" + cityList()}},
			IsError: true,
		}
	}
	if _, ok := cityCoords[city]; !ok {
		return callToolResult{
			Content: []textContent{{Type: "text", Text: "暂不支持查询该城市，当前支持：" + cityList()}},
			IsError: true,
		}
	}
	if queryType == "" {
		queryType = "current"
	}
	if days <= 0 {
		days = 3
	}
	if days > 7 {
		days = 7
	}

	var result string
	switch queryType {
	case "forecast":
		result = buildWeatherForecast(city, days)
	default:
		result = buildCurrentWeather(city)
	}

	log.Printf("[weather_query] city=%s queryType=%s", city, queryType)
	return callToolResult{
		Content: []textContent{{Type: "text", Text: result}},
		IsError: false,
	}
}

func buildCurrentWeather(city string) string {
	today := time.Now()
	w := generateWeather(city, today)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s 今日天气】\n\n", city))
	sb.WriteString(fmt.Sprintf("日期: %s\n", today.Format("2006年01月02日")))
	sb.WriteString(fmt.Sprintf("天气: %s\n", w.WeatherType))
	sb.WriteString(fmt.Sprintf("当前温度: %d°C\n", w.CurrentTemp))
	sb.WriteString(fmt.Sprintf("最高温度: %d°C\n", w.HighTemp))
	sb.WriteString(fmt.Sprintf("最低温度: %d°C\n", w.LowTemp))
	sb.WriteString(fmt.Sprintf("相对湿度: %d%%\n", w.Humidity))
	sb.WriteString(fmt.Sprintf("风向: %s\n", w.WindDirection))
	sb.WriteString(fmt.Sprintf("风力: %s\n", w.WindLevel))
	sb.WriteString(fmt.Sprintf("空气质量: %s\n", w.AirQuality))

	if strings.Contains(w.WeatherType, "雨") || strings.Contains(w.WeatherType, "雪") {
		sb.WriteString("\n提示: 今日有降水，出行请携带雨具。")
	} else if w.HighTemp >= 35 {
		sb.WriteString("\n提示: 今日高温，注意防暑降温。")
	} else if w.LowTemp <= 0 {
		sb.WriteString("\n提示: 今日气温较低，注意防寒保暖。")
	}
	return strings.TrimSpace(sb.String())
}

func buildWeatherForecast(city string, days int) string {
	today := time.Now()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s 未来%d天天气预报】\n\n", city, days))

	for d := 0; d < days; d++ {
		date := today.AddDate(0, 0, d)
		w := generateWeather(city, date)
		var dayLabel string
		switch d {
		case 0:
			dayLabel = "今天"
		case 1:
			dayLabel = "明天"
		case 2:
			dayLabel = "后天"
		default:
			dayLabel = date.Format("01月02日")
		}
		sb.WriteString(fmt.Sprintf("📅 %s（%s）\n", dayLabel, date.Format("01-02")))
		sb.WriteString(fmt.Sprintf("   天气: %s | 温度: %d°C ~ %d°C\n", w.WeatherType, w.LowTemp, w.HighTemp))
		sb.WriteString(fmt.Sprintf("   湿度: %d%% | %s %s\n\n", w.Humidity, w.WindDirection, w.WindLevel))
	}

	todayW := generateWeather(city, today)
	lastW := generateWeather(city, today.AddDate(0, 0, days-1))
	trend := lastW.HighTemp - todayW.HighTemp
	if abs(trend) >= 5 {
		var trendText string
		if trend > 0 {
			trendText = "逐渐升高"
		} else {
			trendText = "逐渐下降"
		}
		var tip string
		if trend > 0 {
			tip = "防暑"
		} else {
			tip = "保暖"
		}
		sb.WriteString(fmt.Sprintf("趋势: 未来%d天气温%s，注意%s。", days, trendText, tip))
	}
	return strings.TrimSpace(sb.String())
}

func cityList() string {
	names := make([]string, 0, len(cityCoords))
	for n := range cityCoords {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, "、")
}

// ====================== Utility ======================

func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func intArg(args map[string]any, key string) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case int64:
			return int(n)
		}
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a < b {
		return b
	}
	return a
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// ====================== Main ======================

func main() {
	// 注册 3 个工具
	register(toolDef{
		Name:        "sales_query",
		Description: "查询软件销售数据，支持按地区、时间、产品、销售人员等维度筛选，支持汇总统计、排名、明细列表、趋势分析等多种查询",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"region":      map[string]any{"type": "string", "description": "地区筛选：华东、华南、华北、西南、西北", "enum": salesRegions},
				"period":      map[string]any{"type": "string", "description": "时间段：本月、上月、本季度、上季度、本年", "enum": []string{"本月", "上月", "本季度", "上季度", "本年"}},
				"product":     map[string]any{"type": "string", "description": "产品筛选：企业版、专业版、基础版", "enum": salesProducts},
				"salesPerson": map[string]any{"type": "string", "description": "销售人员姓名"},
				"queryType":   map[string]any{"type": "string", "description": "查询类型：summary/ranking/detail/trend", "enum": []string{"summary", "ranking", "detail", "trend"}},
				"limit":       map[string]any{"type": "integer", "description": "返回记录数限制，默认10"},
			},
		},
	}, handleSalesQuery)

	register(toolDef{
		Name:        "ticket_query",
		Description: "查询客户技术支持工单数据，支持按地区、状态、优先级、产品、客户等维度筛选，支持汇总概览、工单列表、统计分析等多种查询",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"region":       map[string]any{"type": "string", "description": "地区筛选：华东、华南、华北、西南、西北", "enum": ticketRegions},
				"status":       map[string]any{"type": "string", "description": "工单状态：待处理、处理中、已解决、已关闭", "enum": ticketStatuses},
				"priority":     map[string]any{"type": "string", "description": "优先级：紧急、高、中、低", "enum": ticketPriorities},
				"product":      map[string]any{"type": "string", "description": "产品筛选：企业版、专业版、基础版", "enum": ticketProducts},
				"customerName": map[string]any{"type": "string", "description": "客户名称关键字模糊匹配"},
				"queryType":    map[string]any{"type": "string", "description": "查询类型：summary/list/stats", "enum": []string{"summary", "list", "stats"}},
				"limit":        map[string]any{"type": "integer", "description": "返回记录数限制，默认10"},
			},
		},
	}, handleTicketQuery)

	register(toolDef{
		Name:        "weather_query",
		Description: "查询城市天气信息，支持查看当前实时天气和未来多天天气预报，包含温度、湿度、风力、天气状况等信息",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"city"},
			"properties": map[string]any{
				"city":      map[string]any{"type": "string", "description": "城市名称，如北京、上海、广州等"},
				"queryType": map[string]any{"type": "string", "description": "查询类型：current(当前天气)、forecast(未来预报)", "enum": []string{"current", "forecast"}},
				"days":      map[string]any{"type": "integer", "description": "预报天数，仅forecast有效，默认3天，最多7天"},
			},
		},
	}, handleWeatherQuery)

	http.HandleFunc("/mcp", mcpHandler)

	addr := ":9099"
	log.Printf("========================================")
	log.Printf("  goRAGENT MCP Server v0.0.1")
	log.Printf("  监听: %s", addr)
	log.Printf("  已注册工具: %d 个", len(tools))
	for name := range tools {
		log.Printf("    - %s", name)
	}
	log.Printf("========================================")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
