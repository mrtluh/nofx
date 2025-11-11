package trader

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// OKXTrader OKX交易器
type OKXTrader struct {
	ctx        context.Context
	apiKey     string
	secretKey  string
	passphrase string
	testnet    bool
	client     *http.Client
	baseURL    string

	// 余额缓存
	cachedBalance     map[string]interface{}
	balanceCacheTime  time.Time
	balanceCacheMutex sync.RWMutex

	// 持仓缓存
	cachedPositions     []map[string]interface{}
	positionsCacheTime  time.Time
	positionsCacheMutex sync.RWMutex

	// 精度缓存
	symbolPrecision map[string]OKXSymbolPrecision
	precisionMutex  sync.RWMutex

	// 缓存有效期（15秒）
	cacheDuration time.Duration
}

// SymbolPrecision 交易对精度信息
type OKXSymbolPrecision struct {
	PricePrecision    int
	QuantityPrecision int
	TickSize          float64 // 价格步进值
	StepSize          float64 // 数量步进值
	MinSize           float64 // 最小订单量
}

// NewOKXTrader 创建OKX交易器
func NewOKXTrader(apiKey, secretKey, passphrase string, testnet bool) (*OKXTrader, error) {
	if apiKey == "" || secretKey == "" || passphrase == "" {
		return nil, fmt.Errorf("OKX API密钥、密钥和Passphrase不能为空")
	}

	baseURL := "https://www.okx.com"
	if testnet {
		// OKX测试网使用相同的域名，但可能需要不同的API路径
		log.Printf("⚠️ OKX测试网模式，请确认测试网配置是否正确")
	}

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	log.Printf("✓ OKX交易器初始化成功 (testnet=%v)", testnet)
	return &OKXTrader{
		ctx:             context.Background(),
		apiKey:          apiKey,
		secretKey:       secretKey,
		passphrase:      passphrase,
		testnet:         testnet,
		client:          client,
		baseURL:         baseURL,
		symbolPrecision: make(map[string]OKXSymbolPrecision),
		cacheDuration:   15 * time.Second,
	}, nil
}

// convertSymbol 转换交易对格式：BTCUSDT -> BTC-USDT-SWAP
func (t *OKXTrader) convertSymbol(symbol string) string {
	// 如果已经是 OKX 格式，直接返回
	if strings.Contains(symbol, "-") {
		return symbol
	}

	// 移除 USDT 后缀
	base := strings.TrimSuffix(symbol, "USDT")
	if base == symbol {
		// 如果没有 USDT 后缀，尝试其他常见后缀
		base = strings.TrimSuffix(symbol, "USD")
		if base == symbol {
			return symbol + "-USDT-SWAP" // 默认添加 USDT-SWAP
		}
		return base + "-USD-SWAP"
	}

	return base + "-USDT-SWAP"
}

// reverseSymbol 反向转换：BTC-USDT-SWAP -> BTCUSDT
func (t *OKXTrader) reverseSymbol(okxSymbol string) string {
	// 移除 -SWAP 后缀
	symbol := strings.TrimSuffix(okxSymbol, "-SWAP")
	// 将 - 替换为空
	symbol = strings.ReplaceAll(symbol, "-", "")
	return symbol
}

// generateSignature 生成OKX API签名
func (t *OKXTrader) generateSignature(timestamp, method, requestPath, body string) string {
	// 构建签名字符串
	message := timestamp + method + requestPath + body

	// 使用 HMAC-SHA256 签名
	mac := hmac.New(sha256.New, []byte(t.secretKey))
	mac.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return signature
}

// makeRequest 发送OKX API请求
func (t *OKXTrader) makeRequest(method, endpoint string, body map[string]interface{}) ([]byte, error) {
	var bodyStr string
	var bodyBytes []byte
	var err error

	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("JSON序列化失败: %w", err)
		}
		bodyStr = string(bodyBytes)
	}

	// 构建完整URL
	fullURL := t.baseURL + endpoint

	// 生成时间戳（ISO 8601格式）
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	// 生成签名
	signature := t.generateSignature(timestamp, method, endpoint, bodyStr)

	// 创建请求
	req, err := http.NewRequest(method, fullURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OK-ACCESS-KEY", t.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", timestamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", t.passphrase)

	// 发送请求（带重试机制）
	maxRetries := 3
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP请求失败: %w", err)
			if attempt < maxRetries && (strings.Contains(err.Error(), "timeout") ||
				strings.Contains(err.Error(), "connection reset") ||
				strings.Contains(err.Error(), "EOF")) {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			return nil, lastErr
		}

		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			}
			if json.Unmarshal(respBody, &errResp) == nil {
				return nil, fmt.Errorf("OKX API错误 [%s]: %s", errResp.Code, errResp.Msg)
			}
			return nil, fmt.Errorf("HTTP错误 %d: %s", resp.StatusCode, string(respBody))
		}

		// 解析响应
		var okxResp struct {
			Code string          `json:"code"`
			Msg  string          `json:"msg"`
			Data json.RawMessage `json:"data"`
		}

		if err := json.Unmarshal(respBody, &okxResp); err != nil {
			return nil, fmt.Errorf("解析响应失败: %w", err)
		}

		if okxResp.Code != "0" {
			return nil, fmt.Errorf("OKX API错误 [%s]: %s", okxResp.Code, okxResp.Msg)
		}

		return okxResp.Data, nil
	}

	return nil, fmt.Errorf("请求失败（已重试%d次）: %w", maxRetries, lastErr)
}

// InstrumentInfo 合约规格信息
type InstrumentInfo struct {
	InstID   string `json:"instId"`
	LotSz    string `json:"lotSz"`    // 数量精度（合约张数步进）
	TickSz   string `json:"tickSz"`   // 价格精度
	MinSz    string `json:"minSz"`    // 最小订单量
	Sz       string `json:"sz"`       // 合约面值
	BaseCcy  string `json:"baseCcy"`  // 基础币种
	QuoteCcy string `json:"quoteCcy"` // 计价币种
	InstType string `json:"instType"` // 合约类型
	State    string `json:"state"`    // 状态
}

// getInstrumentInfo 获取合约规格信息
func (t *OKXTrader) getInstrumentInfo(symbol string) (*InstrumentInfo, error) {
	okxSymbol := t.convertSymbol(symbol)

	// 获取交易对信息
	data, err := t.makeRequest("GET", "/api/v5/public/instruments?instType=SWAP&instId="+okxSymbol, nil)
	if err != nil {
		return nil, err
	}

	var instruments []InstrumentInfo

	if err := json.Unmarshal(data, &instruments); err != nil {
		return nil, fmt.Errorf("解析交易对信息失败: %w", err)
	}

	if len(instruments) == 0 {
		return nil, fmt.Errorf("未找到交易对 %s", okxSymbol)
	}

	return &instruments[0], nil
}

// getPrecision 获取交易对精度信息（带缓存）
func (t *OKXTrader) getPrecision(symbol string) (OKXSymbolPrecision, error) {
	okxSymbol := t.convertSymbol(symbol)

	t.precisionMutex.RLock()
	if prec, ok := t.symbolPrecision[okxSymbol]; ok {
		t.precisionMutex.RUnlock()
		return prec, nil
	}
	t.precisionMutex.RUnlock()

	// 获取合约规格信息
	info, err := t.getInstrumentInfo(symbol)
	if err != nil {
		return OKXSymbolPrecision{}, err
	}

	// 解析精度
	lotSz, _ := strconv.ParseFloat(info.LotSz, 64)
	tickSz, _ := strconv.ParseFloat(info.TickSz, 64)
	minSz, _ := strconv.ParseFloat(info.MinSz, 64)

	// 计算精度位数
	quantityPrecision := 0
	if lotSz > 0 {
		quantityPrecision = len(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.10f", lotSz), "0"), "."))
	}

	pricePrecision := 0
	if tickSz > 0 {
		pricePrecision = len(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.10f", tickSz), "0"), "."))
	}

	prec := OKXSymbolPrecision{
		PricePrecision:    pricePrecision,
		QuantityPrecision: quantityPrecision,
		TickSize:          tickSz,
		StepSize:          lotSz,
		MinSize:           minSz,
	}

	// 缓存精度信息
	t.precisionMutex.Lock()
	t.symbolPrecision[okxSymbol] = prec
	t.precisionMutex.Unlock()

	return prec, nil
}

// FormatQuantity 根据合约规格格式化数量
func (t *OKXTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	// 如果数量为0，返回错误（应该在调用前获取实际持仓数量）
	if quantity <= 0 {
		return "", fmt.Errorf("数量必须大于0")
	}

	// 获取合约规格
	info, err := t.getInstrumentInfo(symbol)
	if err != nil {
		// 如果获取失败，使用默认精度（fallback）
		log.Printf("⚠️  获取合约规格失败，使用默认精度: %v", err)
		if strings.Contains(symbol, "BTC") || strings.Contains(symbol, "ETH") {
			return strconv.FormatFloat(math.Round(quantity*100)/100, 'f', 2, 64), nil
		}
		return strconv.FormatFloat(math.Round(quantity*1000)/1000, 'f', 3, 64), nil
	}

	// 解析最小下单数量
	minSz, _ := strconv.ParseFloat(info.MinSz, 64)
	if minSz <= 0 {
		minSz = 1.0 // 默认最小值
	}

	// OKX的sz参数通常是合约张数，需要满足最小下单要求
	// 如果quantity小于最小值，使用最小值
	if quantity < minSz {
		log.Printf("⚠️  数量 %.8f 小于最小下单数量 %.8f，使用最小值", quantity, minSz)
		quantity = minSz
	}

	// 根据lotSz格式化（通常是整数）
	lotSz, _ := strconv.ParseFloat(info.LotSz, 64)
	if lotSz > 0 {
		// 向下取整到lotSz的倍数
		quantity = math.Floor(quantity/lotSz) * lotSz
		if quantity < minSz {
			quantity = minSz
		}
	}

	// 格式化精度（根据合约规格，通常为整数或小数）
	// 大多数OKX永续合约的sz是整数（合约张数）
	return strconv.FormatFloat(math.Floor(quantity), 'f', 0, 64), nil
}

// formatPrice 格式化价格到正确的精度
func (t *OKXTrader) formatPrice(symbol string, price float64) (string, error) {
	prec, err := t.getPrecision(symbol)
	if err != nil {
		return "", err
	}

	// 使用 tick size 进行舍入
	var formatted float64
	if prec.TickSize > 0 {
		// 计算有多少个 tick size
		steps := price / prec.TickSize
		// 向下取整（floor）
		formatted = math.Floor(steps) * prec.TickSize
	} else {
		// 如果没有 tick size，按精度四舍五入
		multiplier := math.Pow10(prec.PricePrecision)
		formatted = math.Floor(price*multiplier) / multiplier
	}

	// 格式化为字符串，去除末尾的0
	result := strconv.FormatFloat(formatted, 'f', prec.PricePrecision, 64)
	result = strings.TrimRight(result, "0")
	result = strings.TrimRight(result, ".")

	return result, nil
}

// GetBalance 获取账户余额（带缓存）
func (t *OKXTrader) GetBalance() (map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.balanceCacheMutex.RLock()
	if t.cachedBalance != nil && time.Since(t.balanceCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.balanceCacheTime)
		t.balanceCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的账户余额（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedBalance, nil
	}
	t.balanceCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用OKX API获取账户余额...")
	data, err := t.makeRequest("GET", "/api/v5/account/balance?ccy=USDT", nil)
	if err != nil {
		return nil, fmt.Errorf("获取账户余额失败: %w", err)
	}

	var balances []struct {
		Details []struct {
			Ccy         string `json:"ccy"`
			Bal         string `json:"bal"`         // 余额
			AvailBal    string `json:"availBal"`    // 可用余额
			FrozenBal   string `json:"frozenBal"`   // 冻结余额
			Eq          string `json:"eq"`          // 权益
			AvailEq     string `json:"availEq"`     // 可用权益
			NotionalUsd string `json:"notionalUsd"` // 美元价值
			OrdFrozen   string `json:"ordFrozen"`   // 挂单冻结
			Upl         string `json:"upl"`         // 未实现盈亏
			MarginRatio string `json:"marginRatio"` // 保证金率
			MgnRatio    string `json:"mgnRatio"`    // 保证金率
		} `json:"details"`
		TotalEq string `json:"totalEq"` // 总权益
	}

	if err := json.Unmarshal(data, &balances); err != nil {
		return nil, fmt.Errorf("解析余额数据失败: %w", err)
	}

	if len(balances) == 0 || len(balances[0].Details) == 0 {
		return nil, fmt.Errorf("未找到账户余额信息")
	}

	detail := balances[0].Details[0]
	totalEq, _ := strconv.ParseFloat(balances[0].TotalEq, 64)
	availEq, _ := strconv.ParseFloat(detail.AvailEq, 64)
	eq, _ := strconv.ParseFloat(detail.Eq, 64)
	bal, _ := strconv.ParseFloat(detail.Bal, 64)
	availBal, _ := strconv.ParseFloat(detail.AvailBal, 64)
	upl, _ := strconv.ParseFloat(detail.Upl, 64)

	// 计算钱包余额（不含未实现盈亏）= 总权益 - 未实现盈亏
	totalWalletBalance := totalEq - upl

	// 返回与Binance相同的字段名，确保AutoTrader能正确解析
	result := map[string]interface{}{
		"totalWalletBalance":    totalWalletBalance, // 钱包余额（不含未实现盈亏）
		"availableBalance":      availEq,            // 可用余额
		"totalUnrealizedProfit": upl,                // 未实现盈亏
		// 兼容字段
		"total_balance":         totalEq,
		"available_balance":     availEq,
		"balance":               bal,
		"available_balance_ccy": availBal,
		"equity":                eq,
		"total_equity":          totalEq,
	}

	log.Printf("✓ OKX API返回: 总权益=%.2f, 钱包余额=%.2f, 未实现盈亏=%.2f, 可用余额=%.2f",
		totalEq, totalWalletBalance, upl, availEq)

	// 更新缓存
	t.balanceCacheMutex.Lock()
	t.cachedBalance = result
	t.balanceCacheTime = time.Now()
	t.balanceCacheMutex.Unlock()

	return result, nil
}

// GetPositions 获取所有持仓（带缓存）
func (t *OKXTrader) GetPositions() ([]map[string]interface{}, error) {
	// 先检查缓存是否有效
	t.positionsCacheMutex.RLock()
	if t.cachedPositions != nil && time.Since(t.positionsCacheTime) < t.cacheDuration {
		cacheAge := time.Since(t.positionsCacheTime)
		t.positionsCacheMutex.RUnlock()
		log.Printf("✓ 使用缓存的持仓信息（缓存时间: %.1f秒前）", cacheAge.Seconds())
		return t.cachedPositions, nil
	}
	t.positionsCacheMutex.RUnlock()

	// 缓存过期或不存在，调用API
	log.Printf("🔄 缓存过期，正在调用OKX API获取持仓信息...")
	data, err := t.makeRequest("GET", "/api/v5/account/positions", nil)
	if err != nil {
		return nil, fmt.Errorf("获取持仓失败: %w", err)
	}

	// 调试：打印原始响应数据
	log.Printf("📥 OKX API原始响应数据: %s", string(data))

	var positions []struct {
		InstID      string `json:"instId"`      // 交易对
		Pos         string `json:"pos"`         // 持仓数量（正数=多仓，负数=空仓）
		AvgPx       string `json:"avgPx"`       // 平均价格
		MarkPx      string `json:"markPx"`      // 标记价格
		LiqPx       string `json:"liqPx"`       // 强平价格
		Upl         string `json:"upl"`         // 未实现盈亏
		UplRatio    string `json:"uplRatio"`    // 未实现盈亏率
		Margin      string `json:"margin"`      // 保证金
		MgnRatio    string `json:"mgnRatio"`    // 保证金率
		Lever       string `json:"lever"`       // 杠杆倍数
		PosSide     string `json:"posSide"`     // 持仓方向：net（净持仓）或 long/short
		MgnMode     string `json:"mgnMode"`     // 保证金模式：isolated（逐仓）或 cross（全仓）
		NotionalUsd string `json:"notionalUsd"` // 美元价值
		Pnl         string `json:"pnl"`         // 已实现盈亏
		PnlRatio    string `json:"pnlRatio"`    // 已实现盈亏率
	}

	if err := json.Unmarshal(data, &positions); err != nil {
		log.Printf("❌ 解析持仓数据失败: %v, 原始数据: %s", err, string(data))
		return nil, fmt.Errorf("解析持仓数据失败: %w", err)
	}

	log.Printf("📊 OKX API返回 %d 个持仓记录", len(positions))

	// 如果返回空数组，打印提示
	if len(positions) == 0 {
		log.Printf("ℹ️ OKX账户当前没有持仓（返回空数组可能是正常的）")
	}

	var result []map[string]interface{}
	for i, pos := range positions {
		log.Printf("  🔍 处理第 %d 个持仓记录: InstID=%s, Pos=%s", i+1, pos.InstID, pos.Pos)

		posAmt, _ := strconv.ParseFloat(pos.Pos, 64)
		if posAmt == 0 {
			log.Printf("  ⏭️ 跳过持仓数量为0的记录: %s", pos.InstID)
			continue // 跳过无持仓的
		}

		// 确定持仓方向
		side := "long"
		if posAmt < 0 {
			side = "short"
			posAmt = -posAmt // 转为正数
		}

		avgPx, _ := strconv.ParseFloat(pos.AvgPx, 64)
		markPx, _ := strconv.ParseFloat(pos.MarkPx, 64)
		upl, _ := strconv.ParseFloat(pos.Upl, 64)
		lever, _ := strconv.ParseFloat(pos.Lever, 64)
		margin, _ := strconv.ParseFloat(pos.Margin, 64)
		notionalUsd, _ := strconv.ParseFloat(pos.NotionalUsd, 64)
		liqPx, _ := strconv.ParseFloat(pos.LiqPx, 64)

		// 转换交易对格式
		symbol := t.reverseSymbol(pos.InstID)

		log.Printf("  📊 OKX持仓: %s (%s) %s %.4f @ %.2f (盈亏: %.2f, 杠杆: %.0fx)",
			symbol, pos.InstID, side, posAmt, avgPx, upl, lever)

		result = append(result, map[string]interface{}{
			"symbol":           symbol,
			"positionAmt":      posAmt,
			"entryPrice":       avgPx,
			"markPrice":        markPx,
			"unRealizedProfit": upl, // 注意：与Binance字段名一致（大写的R）
			"unrealizedPnl":    upl, // 兼容字段
			"leverage":         lever,
			"margin":           margin,
			"notional":         notionalUsd,
			"liquidationPrice": liqPx, // 强平价格
			"side":             side,
			"positionSide":     side, // OKX 使用 posSide，但为兼容性添加 positionSide
			"marginMode":       pos.MgnMode,
			"marginType":       pos.MgnMode, // 兼容性字段
		})
	}

	// 更新缓存
	t.positionsCacheMutex.Lock()
	t.cachedPositions = result
	t.positionsCacheTime = time.Now()
	t.positionsCacheMutex.Unlock()

	log.Printf("✅ OKX持仓获取成功: 共 %d 个持仓", len(result))
	return result, nil
}

// SetMarginMode 设置仓位模式
// 注意：OKX 的仓位模式是在订单参数中指定的，此方法主要用于记录和兼容接口
func (t *OKXTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	marginModeStr := "逐仓"
	if isCrossMargin {
		marginModeStr = "全仓"
	}

	// OKX 的仓位模式通过订单的 tdMode 参数指定，不需要单独设置
	// 这里只是记录日志，实际模式会在下单时通过 tdMode 参数指定
	log.Printf("  ✓ %s 仓位模式将使用 %s (在下单时通过 tdMode 参数指定)", symbol, marginModeStr)
	return nil
}

// SetLeverage 设置杠杆
func (t *OKXTrader) SetLeverage(symbol string, leverage int) error {
	okxSymbol := t.convertSymbol(symbol)

	// 先获取当前持仓信息，检查杠杆是否已经是目标值
	positions, err := t.GetPositions()
	if err == nil {
		for _, pos := range positions {
			if pos["symbol"] == symbol {
				if lev, ok := pos["leverage"].(float64); ok {
					if int(lev) == leverage {
						log.Printf("  ✓ %s 杠杆已是 %dx，无需切换", symbol, leverage)
						return nil
					}
				}
				break
			}
		}
	}

	// 构建请求参数
	// OKX 设置杠杆时需要指定仓位模式，默认使用全仓模式
	// 如果需要逐仓，需要在订单参数中指定 tdMode
	params := map[string]interface{}{
		"instId":  okxSymbol,
		"lever":   strconv.Itoa(leverage),
		"mgnMode": "cross", // 默认全仓模式
	}

	_, err = t.makeRequest("POST", "/api/v5/account/set-leverage", params)
	if err != nil {
		// 如果错误信息包含"already"或"same"，说明杠杆已经是目标值
		if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "same") {
			log.Printf("  ✓ %s 杠杆已是 %dx", symbol, leverage)
			return nil
		}
		return fmt.Errorf("设置杠杆失败: %w", err)
	}

	log.Printf("  ✓ %s 杠杆已切换为 %dx", symbol, leverage)

	// 切换杠杆后等待3秒（避免冷却期错误）
	log.Printf("  ⏱ 等待3秒冷却期...")
	time.Sleep(3 * time.Second)

	return nil
}

// GetMarketPrice 获取市场价格
func (t *OKXTrader) GetMarketPrice(symbol string) (float64, error) {
	okxSymbol := t.convertSymbol(symbol)

	data, err := t.makeRequest("GET", "/api/v5/market/ticker?instId="+okxSymbol, nil)
	if err != nil {
		return 0, fmt.Errorf("获取市场价格失败: %w", err)
	}

	var tickers []struct {
		InstID string `json:"instId"`
		Last   string `json:"last"`   // 最新成交价
		LastPx string `json:"lastPx"` // 最新成交价
		MarkPx string `json:"markPx"` // 标记价格
		BidPx  string `json:"bidPx"`  // 买一价
		AskPx  string `json:"askPx"`  // 卖一价
	}

	if err := json.Unmarshal(data, &tickers); err != nil {
		return 0, fmt.Errorf("解析价格数据失败: %w", err)
	}

	if len(tickers) == 0 {
		return 0, fmt.Errorf("未找到交易对 %s 的价格信息", okxSymbol)
	}

	price, err := strconv.ParseFloat(tickers[0].Last, 64)
	if err != nil {
		// 如果 Last 为空，尝试使用 MarkPx
		price, err = strconv.ParseFloat(tickers[0].MarkPx, 64)
		if err != nil {
			return 0, fmt.Errorf("解析价格失败: %w", err)
		}
	}

	return price, nil
}

// OpenLong 开多仓
func (t *OKXTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	okxSymbol := t.convertSymbol(symbol)

	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 检查格式化后的数量是否为 0
	quantityFloat, parseErr := strconv.ParseFloat(quantityStr, 64)
	if parseErr != nil || quantityFloat <= 0 {
		return nil, fmt.Errorf("开仓数量过小，格式化后为 0 (原始: %.8f → 格式化: %s)", quantity, quantityStr)
	}

	// 构建订单参数
	params := map[string]interface{}{
		"instId":  okxSymbol,
		"tdMode":  "cross", // 全仓模式，如果需要逐仓则改为 "isolated"
		"side":    "buy",
		"ordType": "market",
		"sz":      quantityStr,
	}

	// 发送订单
	data, err := t.makeRequest("POST", "/api/v5/trade/order", params)
	if err != nil {
		return nil, fmt.Errorf("开多仓失败: %w", err)
	}

	var orderResp []struct {
		OrdId   string `json:"ordId"`   // 订单ID
		ClOrdId string `json:"clOrdId"` // 客户订单ID
		Tag     string `json:"tag"`     // 订单标签
		SCode   string `json:"sCode"`   // 事件执行结果
		SMsg    string `json:"sMsg"`    // 事件执行信息
	}

	if err := json.Unmarshal(data, &orderResp); err != nil {
		return nil, fmt.Errorf("解析订单响应失败: %w", err)
	}

	if len(orderResp) == 0 {
		return nil, fmt.Errorf("订单响应为空")
	}

	order := orderResp[0]
	if order.SCode != "0" {
		return nil, fmt.Errorf("订单失败: %s - %s", order.SCode, order.SMsg)
	}

	log.Printf("✓ 开多仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %s", order.OrdId)

	result := map[string]interface{}{
		"orderId": order.OrdId,
		"symbol":  symbol,
		"status":  "FILLED", // 市价单通常立即成交
	}

	return result, nil
}

// OpenShort 开空仓
func (t *OKXTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	okxSymbol := t.convertSymbol(symbol)

	// 先取消该币种的所有委托单（清理旧的止损止盈单）
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ 取消旧委托单失败（可能没有委托单）: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// 格式化数量到正确精度
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 检查格式化后的数量是否为 0
	quantityFloat, parseErr := strconv.ParseFloat(quantityStr, 64)
	if parseErr != nil || quantityFloat <= 0 {
		return nil, fmt.Errorf("开仓数量过小，格式化后为 0 (原始: %.8f → 格式化: %s)", quantity, quantityStr)
	}

	// 构建订单参数
	params := map[string]interface{}{
		"instId":  okxSymbol,
		"tdMode":  "cross", // 全仓模式
		"side":    "sell",
		"ordType": "market",
		"sz":      quantityStr,
	}

	// 发送订单
	data, err := t.makeRequest("POST", "/api/v5/trade/order", params)
	if err != nil {
		return nil, fmt.Errorf("开空仓失败: %w", err)
	}

	var orderResp []struct {
		OrdId   string `json:"ordId"`
		ClOrdId string `json:"clOrdId"`
		Tag     string `json:"tag"`
		SCode   string `json:"sCode"`
		SMsg    string `json:"sMsg"`
	}

	if err := json.Unmarshal(data, &orderResp); err != nil {
		return nil, fmt.Errorf("解析订单响应失败: %w", err)
	}

	if len(orderResp) == 0 {
		return nil, fmt.Errorf("订单响应为空")
	}

	order := orderResp[0]
	if order.SCode != "0" {
		return nil, fmt.Errorf("订单失败: %s - %s", order.SCode, order.SMsg)
	}

	log.Printf("✓ 开空仓成功: %s 数量: %s", symbol, quantityStr)
	log.Printf("  订单ID: %s", order.OrdId)

	result := map[string]interface{}{
		"orderId": order.OrdId,
		"symbol":  symbol,
		"status":  "FILLED",
	}

	return result, nil
}

// CloseLong 平多仓
func (t *OKXTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	okxSymbol := t.convertSymbol(symbol)

	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有 %s 的多仓持仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 构建订单参数（平多仓 = 卖出 + reduceOnly）
	params := map[string]interface{}{
		"instId":     okxSymbol,
		"tdMode":     "cross",
		"side":       "sell",
		"ordType":    "market",
		"sz":         quantityStr,
		"reduceOnly": true, // 只减仓标识
	}

	// 发送订单
	data, err := t.makeRequest("POST", "/api/v5/trade/order", params)
	if err != nil {
		return nil, fmt.Errorf("平多仓失败: %w", err)
	}

	var orderResp []struct {
		OrdId   string `json:"ordId"`
		ClOrdId string `json:"clOrdId"`
		Tag     string `json:"tag"`
		SCode   string `json:"sCode"`
		SMsg    string `json:"sMsg"`
	}

	if err := json.Unmarshal(data, &orderResp); err != nil {
		return nil, fmt.Errorf("解析订单响应失败: %w", err)
	}

	if len(orderResp) == 0 {
		return nil, fmt.Errorf("订单响应为空")
	}

	order := orderResp[0]
	if order.SCode != "0" {
		return nil, fmt.Errorf("订单失败: %s - %s", order.SCode, order.SMsg)
	}

	log.Printf("✓ 平多仓成功: %s 数量: %s", symbol, quantityStr)

	result := map[string]interface{}{
		"orderId": order.OrdId,
		"symbol":  symbol,
		"status":  "FILLED",
	}

	return result, nil
}

// CloseShort 平空仓
func (t *OKXTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	okxSymbol := t.convertSymbol(symbol)

	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("没有 %s 的空仓持仓", symbol)
		}
	}

	// 格式化数量
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return nil, err
	}

	// 构建订单参数（平空仓 = 买入 + reduceOnly）
	params := map[string]interface{}{
		"instId":     okxSymbol,
		"tdMode":     "cross",
		"side":       "buy",
		"ordType":    "market",
		"sz":         quantityStr,
		"reduceOnly": true, // 只减仓标识
	}

	// 发送订单
	data, err := t.makeRequest("POST", "/api/v5/trade/order", params)
	if err != nil {
		return nil, fmt.Errorf("平空仓失败: %w", err)
	}

	var orderResp []struct {
		OrdId   string `json:"ordId"`
		ClOrdId string `json:"clOrdId"`
		Tag     string `json:"tag"`
		SCode   string `json:"sCode"`
		SMsg    string `json:"sMsg"`
	}

	if err := json.Unmarshal(data, &orderResp); err != nil {
		return nil, fmt.Errorf("解析订单响应失败: %w", err)
	}

	if len(orderResp) == 0 {
		return nil, fmt.Errorf("订单响应为空")
	}

	order := orderResp[0]
	if order.SCode != "0" {
		return nil, fmt.Errorf("订单失败: %s - %s", order.SCode, order.SMsg)
	}

	log.Printf("✓ 平空仓成功: %s 数量: %s", symbol, quantityStr)

	result := map[string]interface{}{
		"orderId": order.OrdId,
		"symbol":  symbol,
		"status":  "FILLED",
	}

	return result, nil
}

// SetStopLoss 设置止损单
func (t *OKXTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	okxSymbol := t.convertSymbol(symbol)

	// 格式化数量和价格
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	stopPriceStr, err := t.formatPrice(symbol, stopPrice)
	if err != nil {
		return err
	}

	// 确定订单方向（止损多仓 = 卖出，止损空仓 = 买入）
	side := "sell"
	if positionSide == "short" {
		side = "buy"
	}

	// 构建条件单参数（止损单使用条件单）
	params := map[string]interface{}{
		"instId":          okxSymbol,
		"tdMode":          "cross",
		"side":            side,
		"ordType":         "conditional", // 条件单
		"sz":              quantityStr,
		"slTriggerPx":     stopPriceStr, // 触发价格
		"slTriggerPxType": "last",       // 触发价格类型：last（最新价）
		"reduceOnly":      true,         // 只减仓
	}

	_, err = t.makeRequest("POST", "/api/v5/trade/order-algo", params)
	if err != nil {
		return fmt.Errorf("设置止损单失败: %w", err)
	}

	log.Printf("✓ %s %s 止损单已设置: 触发价格 %.2f, 数量 %s", symbol, positionSide, stopPrice, quantityStr)
	return nil
}

// SetTakeProfit 设置止盈单
func (t *OKXTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	okxSymbol := t.convertSymbol(symbol)

	// 格式化数量和价格
	quantityStr, err := t.FormatQuantity(symbol, quantity)
	if err != nil {
		return err
	}

	takeProfitPriceStr, err := t.formatPrice(symbol, takeProfitPrice)
	if err != nil {
		return err
	}

	// 确定订单方向（止盈多仓 = 卖出，止盈空仓 = 买入）
	side := "sell"
	if positionSide == "short" {
		side = "buy"
	}

	// 构建条件单参数（止盈单使用条件单）
	params := map[string]interface{}{
		"instId":          okxSymbol,
		"tdMode":          "cross",
		"side":            side,
		"ordType":         "conditional",
		"sz":              quantityStr,
		"tpTriggerPx":     takeProfitPriceStr, // 触发价格
		"tpTriggerPxType": "last",             // 触发价格类型
		"reduceOnly":      true,               // 只减仓
	}

	_, err = t.makeRequest("POST", "/api/v5/trade/order-algo", params)
	if err != nil {
		return fmt.Errorf("设置止盈单失败: %w", err)
	}

	log.Printf("✓ %s %s 止盈单已设置: 触发价格 %.2f, 数量 %s", symbol, positionSide, takeProfitPrice, quantityStr)
	return nil
}

// CancelStopLossOrders 取消止损单
func (t *OKXTrader) CancelStopLossOrders(symbol string) error {
	okxSymbol := t.convertSymbol(symbol)

	// 先获取所有算法订单（条件单）
	data, err := t.makeRequest("GET", "/api/v5/trade/orders-algo-pending?instId="+okxSymbol+"&ordType=conditional", nil)
	if err != nil {
		return fmt.Errorf("获取条件单列表失败: %w", err)
	}

	var orders []struct {
		AlgoId      string `json:"algoId"`      // 算法订单ID
		InstId      string `json:"instId"`      // 交易对
		SlTriggerPx string `json:"slTriggerPx"` // 止损触发价格（如果有则说明是止损单）
		TpTriggerPx string `json:"tpTriggerPx"` // 止盈触发价格（如果有则说明是止盈单）
	}

	if err := json.Unmarshal(data, &orders); err != nil {
		return fmt.Errorf("解析订单列表失败: %w", err)
	}

	// 取消所有止损单
	for _, order := range orders {
		if order.SlTriggerPx != "" && order.SlTriggerPx != "0" {
			// 这是止损单，取消它
			cancelParams := map[string]interface{}{
				"instId":  okxSymbol,
				"algoId":  order.AlgoId,
				"ordType": "conditional",
			}

			_, err := t.makeRequest("POST", "/api/v5/trade/cancel-algo", cancelParams)
			if err != nil {
				log.Printf("  ⚠️ 取消止损单失败 (algoId: %s): %v", order.AlgoId, err)
				continue
			}
			log.Printf("  ✓ 已取消止损单 (algoId: %s)", order.AlgoId)
		}
	}

	return nil
}

// CancelTakeProfitOrders 取消止盈单
func (t *OKXTrader) CancelTakeProfitOrders(symbol string) error {
	okxSymbol := t.convertSymbol(symbol)

	// 先获取所有算法订单（条件单）
	data, err := t.makeRequest("GET", "/api/v5/trade/orders-algo-pending?instId="+okxSymbol+"&ordType=conditional", nil)
	if err != nil {
		return fmt.Errorf("获取条件单列表失败: %w", err)
	}

	var orders []struct {
		AlgoId      string `json:"algoId"`
		InstId      string `json:"instId"`
		SlTriggerPx string `json:"slTriggerPx"`
		TpTriggerPx string `json:"tpTriggerPx"`
	}

	if err := json.Unmarshal(data, &orders); err != nil {
		return fmt.Errorf("解析订单列表失败: %w", err)
	}

	// 取消所有止盈单
	for _, order := range orders {
		if order.TpTriggerPx != "" && order.TpTriggerPx != "0" {
			// 这是止盈单，取消它
			cancelParams := map[string]interface{}{
				"instId":  okxSymbol,
				"algoId":  order.AlgoId,
				"ordType": "conditional",
			}

			_, err := t.makeRequest("POST", "/api/v5/trade/cancel-algo", cancelParams)
			if err != nil {
				log.Printf("  ⚠️ 取消止盈单失败 (algoId: %s): %v", order.AlgoId, err)
				continue
			}
			log.Printf("  ✓ 已取消止盈单 (algoId: %s)", order.AlgoId)
		}
	}

	return nil
}

// CancelAllOrders 取消该币种的所有挂单
func (t *OKXTrader) CancelAllOrders(symbol string) error {
	okxSymbol := t.convertSymbol(symbol)

	// 取消所有普通订单
	cancelParams := map[string]interface{}{
		"instId": okxSymbol,
	}

	_, err := t.makeRequest("POST", "/api/v5/trade/cancel-all-after", cancelParams)
	// 如果失败，尝试使用批量取消接口
	if err != nil {
		// 获取所有待处理订单
		data, err := t.makeRequest("GET", "/api/v5/trade/orders-pending?instId="+okxSymbol, nil)
		if err != nil {
			return fmt.Errorf("获取订单列表失败: %w", err)
		}

		var orders []struct {
			OrdId string `json:"ordId"`
		}

		if err := json.Unmarshal(data, &orders); err != nil {
			return fmt.Errorf("解析订单列表失败: %w", err)
		}

		// 批量取消订单
		for _, order := range orders {
			cancelParams := map[string]interface{}{
				"instId": okxSymbol,
				"ordId":  order.OrdId,
			}

			_, err := t.makeRequest("POST", "/api/v5/trade/cancel-order", cancelParams)
			if err != nil {
				log.Printf("  ⚠️ 取消订单失败 (ordId: %s): %v", order.OrdId, err)
				continue
			}
		}
	}

	// 取消所有条件单（止损止盈单）
	if err := t.CancelStopLossOrders(symbol); err != nil {
		log.Printf("  ⚠️ 取消止损单失败: %v", err)
	}

	if err := t.CancelTakeProfitOrders(symbol); err != nil {
		log.Printf("  ⚠️ 取消止盈单失败: %v", err)
	}

	log.Printf("  ✓ 已取消 %s 的所有挂单", symbol)
	return nil
}

// CancelStopOrders 取消该币种的止盈/止损单
func (t *OKXTrader) CancelStopOrders(symbol string) error {
	// 取消止损单
	if err := t.CancelStopLossOrders(symbol); err != nil {
		log.Printf("  ⚠️ 取消止损单失败: %v", err)
	}

	// 取消止盈单
	if err := t.CancelTakeProfitOrders(symbol); err != nil {
		log.Printf("  ⚠️ 取消止盈单失败: %v", err)
	}

	log.Printf("  ✓ 已取消 %s 的所有止盈/止损单", symbol)
	return nil
}
