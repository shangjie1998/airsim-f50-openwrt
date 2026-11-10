---
name: repo-readme-rename-blanks-github-homepage
description: 仓库结构调整时给 README 加了机型/分支后缀，导致 GitHub 首页说明空白；以及"本地 commit 在"被当成"已推送"时怎么核验远端
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-10
 tags: [github, readme, 仓库规范, 推送核验, 静默失败]
---
## 适用场景
把单仓库改成 `src/`+`overlay/` 分层、或给一个仓库加机型区分时，顺手把 `README.md` 改名成 `README-M8.md` 这类带后缀的名字；以及需要确认"推上去了没有"的时候。

## 关键做法
- 根目录说明文件必须保持标准名：`README.md`（或 `README`、`README.rst`）。GitHub 首页只按这套固定名渲染，任何后缀都当作普通文件，首页直接空白且不报错。需要区分机型就把机型信息写进正文标题，不要写进文件名。
- 已经改过名的，用 `git mv README-M8.md README.md` 改回来，同时核对正文里的版本号、装机示例包名、更新说明段落是否还停留在旧版本。
- 判断"是否真的已推送"，只认远端：`git ls-remote <remote>` 看分支 sha，或 API 回读 `contents/`（根目录 README 文件名）与 `releases?per_page=1`（附件名和字节数）。本地 `git log` 有 commit、工作区干净，都不能证明推上去了。

## 踩过的坑
- 分层重构那次把 README 改名，交付时完全没察觉，直到用户问"我主页说明咋没了"才发现；改名本身不会让任何命令报错。
- 更严重的是口头声称"已提交、已推送、Release 已建"，实际 `main` 还停在上一版 commit、Release 根本不存在。原因是只看了本地 commit 和提交输出，没回读远端。教训：任何"已推送/已发布"的结论都要有远端回读证据，否则先别说。

## 可复用的产物或模板
```
git ls-remote <remote> | head -3                      # 分支 sha 对不对
curl -s -H "Authorization: token $TOKEN" \
  https://api.github.com/repos/<owner>/<repo>/contents/ | \
  python3 -c "import json,sys;[print(x['name']) for x in json.load(sys.stdin) if x['name'].lower().startswith('readme')]"
curl -s -H "Authorization: token $TOKEN" \
  "https://api.github.com/repos/<owner>/<repo>/releases?per_page=1" | \
  python3 -c "import json,sys;r=json.load(sys.stdin)[0];print(r['tag_name'],r['assets'][0]['name'],r['assets'][0]['size'])"
```