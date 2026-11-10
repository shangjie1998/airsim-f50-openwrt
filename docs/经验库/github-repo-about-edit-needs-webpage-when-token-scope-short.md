---
name: github-repo-about-edit-needs-webpage-when-token-scope-short
description: 改 GitHub 仓库 About 简介时 API 报 403 Resource not accessible，怎么改走网页、在新布局里定位编辑入口并确认改上了
metadata:
 type: work_experience
 source: harness
 created_at: 2026-11-09
 tags: [github, pat权限, 仓库元信息, browser-use, 静默失败, 核验]
---
## 适用场景
需要修改 GitHub 仓库主页的 About 简介（description）或 home 链接，而手上的 PAT 只开了 Contents/Workflows 之类有限 scope 的时候。

## 关键做法
1. 先用 `PATCH https://api.github.com/repos/<owner>/<repo>` 带 `{"description": ...}` 试一手，返回 403 且 message 为 `Resource not accessible by personal access token`，即说明这把 token 没有 metadata 权限，直接放弃 API、改走网页，不要反复重试。
2. 打开仓库首页，About 区块右上角那个铅笔按钮在新布局里用 `bu.css('aside')` 匹配不到、`bu.find('Edit repository details')` 也是 0 匹配；改用 JS 按 aria-label 定位：`document.querySelectorAll('button,a')` 里筛 `Edit repository metadata`。
3. 点开弹窗后 Description 是页面上唯一的 textarea，`bu.type(ref, 新文案, submit=False)` 填好后，`bu.find('Save changes')` 取按钮再 click。
4. 保存后**必须 `bu.navigate('reload')` 再读 `bu.get_page_text()`**，用「旧文案是否还在 / 新文案是否出现」两个布尔判断核验，别拿保存前那次快照或保存后未刷新的页面文本当已生效。

## 踩过的坑
- PAT 只有 Contents scope 时仓库元信息 API 恒 403，此路不通；Release、附件上传用同一把 token 却是好的，容易误以为整体权限没问题。
- 新布局把 About 放进了独立的 SidebarAbout 模块，`aside` 这个 CSS 定位不到元素，直接抛 `PY_BROWSER_USE_ERROR: The CSS locator matched no elements`。
- 弹窗关闭后 `get_page_text()` 大概率仍读到旧描述（页面文本未及时更新），只有 reload 之后才读到新值——保存成功与否只能靠刷新后的读回来判。

## 可复用的产物或模板
```python
bu.navigate('https://github.com/<owner>/<repo>')
print(bu.js("Array.from(document.querySelectorAll('button,a')).filter(e=>/edit repository metadata/i.test(e.getAttribute('aria-label')||'')).map(e=>e.getAttribute('aria-label'))"))
bu.click(bu.css('button[aria-label="Edit repository metadata"]', match='first'))
bu.wait_for_element('textarea', visible=True, timeout=8)
bu.type(<textarea ref>, '<新简介>', submit=False)
bu.click(bu.find('Save changes', search_type='interaction', limit=5).first)
bu.navigate('reload'); t = bu.get_page_text(max_chars=2000)
assert '<旧片段>' not in t and '<新片段>' in t
```