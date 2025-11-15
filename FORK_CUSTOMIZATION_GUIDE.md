# 🔧 Fork 專案客製化指南

這份指南幫助你將 Fork 的 NOFX 專案客製化為你自己的版本，包括設置返傭連結以賺取推薦收入。

## 📋 檢查清單

### 🔴 必須修改的項目

#### 1. 返傭連結（賺取推薦佣金）

##### Binance 推薦碼
**位置**: README.md Line 246
**原值**: `TINKLEVIP`
**改為**: 你的 Binance 推薦碼

**如何獲取 Binance 推薦碼：**
1. 登入 Binance
2. 前往「推薦」頁面: https://www.binance.com/en/my/referral
3. 複製你的推薦 ID
4. 替換 README.md 中的連結：
   ```markdown
   **🎁 [Register Binance - Get Fee Discount](https://www.binance.com/join?ref=YOUR_REF_CODE)**
   ```

##### Hyperliquid 推薦碼
**位置**: README.md Line 508
**原值**: `AITRADING`
**改為**: 你的 Hyperliquid 推薦碼

**如何獲取 Hyperliquid 推薦碼：**
1. 登入 Hyperliquid: https://app.hyperliquid.xyz
2. 點擊頭像 → Settings → Referrals
3. 創建你的推薦代碼
4. 替換 README.md 中的連結：
   ```markdown
   **🎁 [Register Hyperliquid - Join YOUR_CODE](https://app.hyperliquid.xyz/join/YOUR_CODE)**
   ```

##### Aster DEX 推薦碼
**位置**: README.md Lines 124, 621
**原值**: `fdfc0e`
**改為**: 你的 Aster 推薦碼

**如何獲取 Aster 推薦碼：**
1. 登入 Aster DEX: https://www.asterdex.com
2. 前往推薦頁面: https://www.asterdex.com/en/referral
3. 複製你的推薦碼
4. 替換 README.md 中的連結：
   ```markdown
   **🎁 [Register Aster DEX](https://www.asterdex.com/en/referral/YOUR_REF_CODE)**
   ```

---

#### 2. Git Clone URL

**位置**: README.md Line 357
**原值**: `https://github.com/tinkle-community/nofx.git`
**改為**: `https://github.com/the-dev-z/nofx.git`

```markdown
git clone https://github.com/the-dev-z/nofx.git
cd nofx
```

---

#### 3. GitHub Issues 連結

**位置**: README.md Line 1349
**原值**: `https://github.com/tinkle-community/nofx/issues`
**改為**: `https://github.com/the-dev-z/nofx/issues`

---

### 🟡 建議修改的項目

#### 4. 移除原作者的投資資訊

**位置**: README.md Lines 51-61

**建議操作**: 移除以下內容：
```markdown
### 🏢 Backed by [Amber.ac](https://amber.ac)

### 👥 Core Team

- **Tinkle** - [@Web3Tinkle](https://x.com/Web3Tinkle)

### 💼 Seed Funding Round Open

We are currently raising our **seed round**.

**For investment inquiries**, please DM **Tinkle** via Twitter.
```

**替換為你自己的資訊**（可選）：
```markdown
### 👥 開發團隊

- **你的名字** - [@你的Twitter](https://x.com/你的帳號)

### 💬 聯繫方式

如有問題或建議，歡迎通過以下方式聯繫：
- GitHub Issues: https://github.com/the-dev-z/nofx/issues
- Telegram: 你的聯繫方式
```

---

#### 5. 移除原專案的 Star History

**位置**: README.md Lines 1370-1372

**建議操作**:
- **選項 A**: 移除這段（因為顯示的是原專案的 Star 數）
- **選項 B**: 替換為你自己的 Fork：
  ```markdown
  [![Star History Chart](https://api.star-history.com/svg?repos=the-dev-z/nofx&type=Date)](https://star-history.com/#the-dev-z/nofx&Date)
  ```

---

#### 6. 更新 Badge（可選）

**位置**: README.md Line 7

**建議操作**: 移除 "Backed by Amber.ac" badge：
```markdown
[![Backed by Amber.ac](https://img.shields.io/badge/Backed%20by-Amber.ac-orange.svg)](https://amber.ac)
```

或替換為你自己的贊助商/支持者。

---

### 🟢 可選修改項目

#### 7. 添加 Fork 聲明

**位置**: README.md 開頭（Line 13 之後）

**建議添加**:
```markdown
---

> 📢 **Fork Notice**: This is a community fork of the original NOFX project. We maintain and improve this version independently while respecting the AGPL-3.0 license.
>
> **Original Repository**: [nofxaios/nofx](https://github.com/nofxaios/nofx)

---
```

---

#### 8. 更新聯繫方式

**位置**: README.md Lines 1345-1350

**建議修改**:
```markdown
## 📬 Contact

### 🐛 Technical Support
- **GitHub Issues**: [Submit an Issue](https://github.com/the-dev-z/nofx/issues)
- **Developer Community**: [你的 Telegram/Discord 群組]（可選）

### 💬 Community
- **Twitter**: [@你的帳號](https://x.com/你的帳號)（可選）
```

---

#### 9. 添加貢獻指引

**位置**: CONTRIBUTING.md（如果你想接受外部貢獻）

**建議內容**:
```markdown
# Contributing to NOFX (Fork)

This is a community fork of NOFX. We welcome contributions!

## How to Contribute

1. Fork this repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

By contributing, you agree that your contributions will be licensed under AGPL-3.0.
```

---

## 🎯 快速修改腳本

我可以幫你創建一個自動替換腳本，只需提供：

1. **你的 Binance 推薦碼**: `YOUR_BINANCE_REF`
2. **你的 Hyperliquid 推薦碼**: `YOUR_HYPERLIQUID_REF`
3. **你的 Aster 推薦碼**: `YOUR_ASTER_REF`
4. **你的名字/團隊名稱**: `YOUR_NAME`
5. **你的聯繫方式**: `YOUR_CONTACT`

然後我可以幫你批量修改所有必要的文件。

---

## ⚖️ License 注意事項

**AGPL-3.0 要求**:
- ✅ 必須保留原始版權聲明
- ✅ 必須公開你的修改（如果你提供服務）
- ✅ 衍生作品必須使用相同的 AGPL-3.0 許可證

**你可以做的**:
- ✅ 商業使用
- ✅ 修改代碼
- ✅ 分發你的版本
- ✅ 使用你自己的返傭連結

**你不能做的**:
- ❌ 改為閉源
- ❌ 移除原始作者的版權聲明
- ❌ 聲稱是原創作品

---

## 📝 修改完成檢查表

- [ ] Binance 推薦碼已更新
- [ ] Hyperliquid 推薦碼已更新
- [ ] Aster 推薦碼已更新
- [ ] Git clone URL 已更新
- [ ] GitHub Issues 連結已更新
- [ ] 移除原作者投資資訊
- [ ] 更新聯繫方式
- [ ] 添加 Fork 聲明（可選）
- [ ] 測試所有推薦連結是否正確

---

**Last Updated**: 2025-11-15
