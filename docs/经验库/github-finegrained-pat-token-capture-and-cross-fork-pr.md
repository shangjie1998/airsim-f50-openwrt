---
name: github-finegrained-pat-token-capture-and-cross-fork-pr
description: 用浏览器建细粒度 PAT 取明文、并给上游仓库开 PR 时，避开明文丢失、有效期被重置和 token 无权限开 PR 三个坑
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-08
 tags: [github, fine-grained-pat, pull-request, browser-use, fork, token]
---
## 适用场景
需要用 GitHub 细粒度令牌向自己的 fork 推送分支、并向非本账号的上游仓库提 PR，同时必须拿到令牌明文时。

## 关键做法
- 建令牌：Only select repositories 只勾目标 fork，权限给 Contents: Read and write；推送前先 `git ls-remote "https://x-access-token:$TOKEN@github.com/<user>/<repo>.git"` 看分支是否已在远端，避免重复推。
- 拿明文：令牌只在一跳页面显示成一个 disabled 的 `Access token` 输入框，`get_page_text` 抓不到。用 `bu.snapshot(view="all")` 找到该 textbox 的 ref，再 `bu.get(ref, "value")` 读出完整 `github_pat_...`。
- 定 base：`GET /repos/<upstream>` 读 `default_branch` 再决定 PR 目标分支，不要假定是 main。
- 开 PR：token 只授权 fork 仓库时，`POST /repos/<upstream>/pulls` 会返回 `Resource not accessible by personal access token`。改走网页 `https://github.com/<upstream>/compare/<base>...<user>:<repo>:<branch>?expand=1`，描述框用 CSS `#pull_request_body` 写入并回读比对全文一致，再点 "Create pull request"，URL 变成 `/pull/<n>` 即成功。

## 踩过的坑
- 生成后没立刻抓明文，刷新或返回列表页后明文永久丢失，且页面不回显。
- 为了补抓明文点 "Regenerate token"，会把有效期重置成默认 30 天，原先选的 7 天丢失——令牌已能用的前提下别轻易 regenerate；非做不可就重新确认 expiration。
- `bu.js()` 返回值不满足 JSON 序列化要求会直接报错，读表单值优先用 snapshot + `bu.get(ref,"value")`。
- 提交按钮常被浮层层遮住报 `BU_TARGET_OBSCURED`，改用 `find()` 返回的浮层内按钮 ref 点击。

## 可复用的产物或模板
```
git ls-remote "https://x-access-token:$TOKEN@github.com/<user>/<repo>.git"
curl -s -H "Authorization: Bearer $TOKEN" https://api.github.com/repos/<upstream>  # 读 default_branch
https://github.com/<upstream>/compare/<base>...<user>:<repo>:<branch>?expand=1
```