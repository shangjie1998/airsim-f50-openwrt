---
name: github-push-403-finegrained-pat-workflows
description: 本地 git push 到 GitHub 被 403 时用浏览器建 fine-grained PAT 打通上传；以及发布 Release 时附件上传"成功却为空"的判定与修法
metadata:
    type: work_experience
    source: harness
    created_at: "2026-11-08"
    tags:
        - git
        - github
        - "403"
        - fine-grained-pat
        - workflows
        - release-asset
        - 权限
        - 文件上传
---
## 适用场景
- `git push` 返回 `remote: Permission to <owner>/<repo>.git denied to <user>` + `403`，但 `git credential fill` 明明有 username/password。
- 需要给指定仓库开最小权限、可自动过期的上传凭据；或后续要给第二个仓库用同一张 token。
- 网页发布 GitHub Release，点了上传却拿不到附件（下载 404、API assets 为空）。

## 关键做法
1. `403 denied to <user>` 且用户名就是账号本人，说明 token 有效但 scope/仓库授权缺，不要在本地反复重试或改走 SSH。
2. 浏览器建 fine-grained PAT：Settings → Developer settings → Personal access tokens → Fine-grained tokens → Generate new token。必过 sudo mode（"Confirm access"），属认证检查点，交用户接管；回来后 `bu.resync()` 再重新 `snapshot()`，旧 ref 全废。表单顺序：name → expiration → Resource owner → **Only select repositories** → 在选择器里 Filter 出目标仓库点选（选中后按钮变 `Select repositories N` 并出现 `Remove <repo>` 才算真选上）→ Add permissions。权限至少两项：**Contents: Read and write**（推对象）和 **Workflows: Read and write**（仓库含 `.github/workflows` 时，触及 workflow 文件的提交会被拒）。`Add permissions` 展开后选项在 listbox 里、常标 `[not-visible]`，用 `bu.find('Contents', search_type='interaction')` 拿 ref 直接点更稳。刚加的权限默认 **Read-only**，要再点 `Access: Read-only` 改成 `Read and write`；点前确认浮层已收起，否则被遮挡。
3. 新增仓库复用同一张 token：进 `/settings/personal-access-tokens/<id>` → Edit → `Repositories N` 展开 → Filter 出新仓库点选 → Close → 点 Update。页面出现 "has been updated" 即生效，无需重建 token。
4. 点 `Generate token` 会出现**确认弹窗**，弹窗里还有第二个同名按钮。按文本 find 会命中页面外层那个（点了停在表单，只读到 CSRF hidden 值）。必须读快照区分：外层在 `form` 内、弹窗那个挂在 form 之外的 box 里。token 只显示一次，**必须在点击的同一 cell 里立刻读值**（`bu.read_all('input,textarea', fields=['type','value'])` 过滤 `github_pat` 前缀），或同 cell 内 `bu.js` 取所有 input 的 value；页面会立刻跳回列表，下一轮就取不到了。
5. 推送用一次性 URL，推完立刻摘掉：
   ```
   TOKEN=$(cat /tmp/<file>)
   git remote add <name> "https://x-access-token:$TOKEN@github.com/<owner>/<repo>.git"
   git push <name> <local>:<remote-branch>
   git remote set-url <name> https://github.com/<owner>/<repo>.git
   grep -c 'x-access-token' .git/config    # 必须为 0
   ```
6. Release 附件：**文件名里不要有括号**，用连字符代替 `()`；显示名与仓库里的原始包名可以不同，只要内容一致。上传要打 Assets 那个文件输入（`input#releases-upload`），**不要用 Release 正文编辑器的附件输入**——后者只是把 `user-attachments` 链接插进正文，不会生成 release asset。
7. 发布后核验看 API 而不是页面文字：
   ```
   curl -s https://api.github.com/repos/<owner>/<repo>/releases/latest
   # assets[].name / size 要有值
   curl -sIL -o /dev/null -w '%{http_code}' https://github.com/<owner>/<repo>/releases/download/<tag>/<asset>
   ```

## 踩过的坑
- Release 首次发布后页面看着正常，但 `api .../releases/latest` 的 assets 是空数组、`releases/download/...` 返回 404——**发布成功不等于附件上传成功**。回到 `releases/edit/<tag>`，Assets 区会明写 `<文件名> Upload failed. Delete and try uploading this file again.` 先点 × 删掉残项再重传。
- 原名含 `(1)` 的包会被 GitHub 把括号换成点（`airsim-m8-0.1.0.1.-r22-m8.tar.gz`）并报 Upload failed；改成 `airsim-m8-0.1.0-1-r22-m8.tar.gz` 一次成功。
- 用正文编辑器的附件输入上传，得到的是 `[name](https://github.com/user-attachments/files/...)` 这样插进描述里的链接，资产列表依旧为空，且会污染正文——之后要先清掉这行再重写正文。
- 只给 Contents 权限、仓库又带 `.github/workflows` 时会再 403；补 Workflows 一次过。
- `.gitignore` 里要让成品包仍被跟踪，规则必须是 `/dist/*` + `!/dist/xxx.tar.gz`；只写 `/dist/` 再否定单个文件无效（目录整体被排除，否定规则看不到路径）。
- 只 `curl -r 0-1000` 测 Range 可能回 0 字节，不能据此判断资源为空；改成整包下载比 md5。

## 可复用的产物或模板
（无）