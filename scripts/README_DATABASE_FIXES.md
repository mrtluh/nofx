# 數據庫修復腳本使用指南

## 問題診斷

### ❌ 症狀：「交易所X不存在」錯誤

當您看到以下錯誤日誌時：

```
⚠️  交易员 DeepSeek AI-Binance-1 的交易所 7 不存在，跳过
成功加载 0 个交易员到内存
```

**原因**：traders 表中的 `exchange_id` 引用了不存在的 exchanges.id（外鍵約束失效）

---

## 🛠️ 修復方法

### 方法 1：自動修復（推薦）

使用我們提供的自動診斷和修復腳本：

#### 步驟 1：進入 Docker 容器

```bash
docker exec -it nofx-api-1 bash
```

#### 步驟 2：執行修復腳本

```bash
# 容器內執行
cd /app/scripts
./fix_missing_exchange_references.sh
```

腳本會：
1. 🔍 診斷數據庫（顯示所有孤立的 trader 記錄）
2. 💾 自動備份數據庫
3. 🗑️ 刪除孤立的 trader 記錄（引用不存在的交易所）
4. ✅ 驗證修復結果

#### 步驟 3：重啟容器

```bash
exit  # 退出容器
docker-compose restart
```

---

### 方法 2：手動診斷（高級用戶）

如果您想先檢查數據庫狀態：

```bash
# 進入容器
docker exec -it nofx-api-1 bash

# 查看所有交易所
sqlite3 /data/nofx.db "SELECT id, exchange_id, name FROM exchanges;"

# 查看孤立的 traders
sqlite3 /data/nofx.db "
SELECT t.id, t.name, t.exchange_id
FROM traders t
WHERE NOT EXISTS (
    SELECT 1 FROM exchanges e WHERE e.id = t.exchange_id
);
"
```

#### 手動刪除孤立記錄

```bash
sqlite3 /data/nofx.db <<EOF
DELETE FROM traders
WHERE id IN (
    SELECT t.id
    FROM traders t
    WHERE NOT EXISTS (
        SELECT 1 FROM exchanges e WHERE e.id = t.exchange_id
    )
);
EOF
```

---

## 🔍 其他可用的修復腳本

### 1. `fix_exchanges_schema.sh`

**用途**：修復 exchanges 表結構（id vs exchange_id 列混淆）

**何時使用**：
- 升級舊版本後，exchanges 表結構不正確
- 看到 "column exchange_id not found" 錯誤

```bash
docker exec -it nofx-api-1 bash
cd /app/scripts
./fix_exchanges_schema.sh
```

### 2. `fix_traders_table_migration.sh`

**用途**：修復 traders 表結構和外鍵引用

**何時使用**：
- traders 表缺少新增的列（如 timeframes, order_strategy）
- 外鍵引用混亂

```bash
docker exec -it nofx-api-1 bash
cd /app/scripts
./fix_traders_table_migration.sh
```

---

## ⚠️ 預防措施

### 避免未來出現此問題：

1. **不要直接刪除交易所配置**
   如果需要停用交易所，請使用 `enabled=0` 而不是刪除記錄

2. **刪除交易員前檢查**
   確保沒有運行中的交易員引用該交易所

3. **定期備份數據庫**
   ```bash
   docker exec nofx-api-1 cp /data/nofx.db /data/nofx.db.backup.$(date +%Y%m%d)
   ```

4. **查看日誌**
   ```bash
   docker-compose logs -f api | grep "交易所.*不存在"
   ```

---

## 📞 需要幫助？

如果自動修復腳本無法解決問題，請：

1. 保留備份文件（`nofx.db.backup.*`）
2. 記錄錯誤日誌
3. 檢查 traders 和 exchanges 表的內容：
   ```bash
   docker exec -it nofx-api-1 sqlite3 /data/nofx.db ".dump traders"
   docker exec -it nofx-api-1 sqlite3 /data/nofx.db ".dump exchanges"
   ```

---

## 📝 修復日誌

| 日期 | 腳本 | 說明 |
|------|------|------|
| 2025-11-15 | `fix_missing_exchange_references.sh` | 新增：修復孤立的 trader 記錄 |
| 2025-01-XX | `fix_traders_table_migration.sh` | traders 表結構遷移 |
| 2025-01-XX | `fix_exchanges_schema.sh` | exchanges 表結構修復 |

---

**生成者**: Claude Code
**版本**: 1.0
**更新日期**: 2025-11-15
