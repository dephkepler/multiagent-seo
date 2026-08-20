# Skills — кто есть кто

Не читается Claude Code как skill (не `SKILL.md`) — чисто индекс для навигации. Каждый реальный
skill подписан кодовым именем первой строкой в своём `SKILL.md`; команда вызова (`/имя-папки`) от
кодового имени не зависит — берётся из имени директории.

| Кодовое имя       | Skill                                       | Вызов                   | Что делает |
|--------------------|----------------------------------------------|--------------------------|------------|
| Захар Палыч        | [`deploy`](deploy/SKILL.md)                   | `/deploy`                | Прод-раскатка multiagent-seo (SSH, docker compose) |
| Нина Аркадьевна     | [`code-standards-review`](code-standards-review/SKILL.md) | `/code-standards-review` | Систематический проход по `doc/audit/*.md` до прод-состояния |

Общие/личные скилы (`comments-cleanup`, `review-agent-work`, `commit`, `push`, `deploy-vps`, `en`,
`obs`) переехали в `~/pet-projects/claude-toolkit` — симлинкнуты в `~/.claude/skills/`, доступны
во всех проектах на этой машине, здесь больше не лежат.

Новый **project-local** skill — добавляй строкой сюда же.
