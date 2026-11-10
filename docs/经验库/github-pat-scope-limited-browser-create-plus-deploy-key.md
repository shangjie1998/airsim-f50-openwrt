---
name: github-pat-scope-limited-browser-create-plus-deploy-key
description: 现有 GitHub token 只能推某个老仓库、建新仓库和推新仓库都 403 时，怎么不换 token 就把新仓库建出来并推上去
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [github, pat, 部署密钥, 权限边界, 浏览器操作, git-push]
---
## 适用场景
要新建一个 GitHub 仓库并推内容，但手上 token 的细粒度权限只覆盖已有仓库：`POST /user/repos` 回 403 `Resource not accessible by personal access token`，用同一个 token 推新仓库地址也回 403 `Permission to <owner>/<repo>.git denied`。

## 关键做法
1. 用浏览器打开 `https://github.com/new` 手动建仓：只填仓库名、描述、选 Public，README/gitignore/license 全部留空，得到空仓库。
2. 在本地生成一把专用密钥：`ssh-keygen -t ed25519 -f ~/.ssh/<name>_deploy -N ''`。
3. 浏览器进 `<repo>/settings/keys` → Add deploy key → 填 Title 与公钥 → 勾 **Allow write access** → Add key。
4. 加写权限的部署密钥属于 sudo 保护操作，会跳 `Confirm access` 二次验证页，必须交用户完成，别在旁边试别的路。
5. 验证完成后用 `GIT_SSH_COMMAND="ssh -i ~/.ssh/<name>_deploy -o StrictHostKeyChecking=no" git push git@github.com:<owner>/<repo>.git HEAD:main`，成功后 `git remote add origin` 记下来。

## 踩过的坑
- 先试 `POST /user/repos` 直接建仓，403；同一 token 推新仓库也 403——token 权限是**按仓库**给的，不是按账号，建仓和写新仓都要单独授权。
- 加部署密钥这一步不走完 sudo 验证，密钥不会落地，`ssh -T git@github.com` 会回 `Permission denied (publickey)`。
- 别先怀疑网络或 SSH 配置；先看是密钥没加进去，还是没勾 write access。

## 可复用的产物或模板
（无）