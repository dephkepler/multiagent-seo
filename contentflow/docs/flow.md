# Contentflow — Pipeline

## Флоу

```
1. Пользователь → Telegram
   Отправляет тему, например: "лучшие кофемашины 2024"

2. Keyword Matcher
   Ищем подходящие ключевые слова из Google Sheets под эту тему
   (Claude подбирает наиболее релевантные)

3. Brief Agent (Claude)
   Генерирует ТЗ статьи: структура, H2/H3, ключевые тезисы, целевая аудитория

4. Writer Agent (Claude)
   Пишет статью по ТЗ

5. Editor Agent (Claude)
   Правит: SEO, читабельность, стиль

6. WordPress
   Сохраняем статью как ЧЕРНОВИК (не виден на сайте)

7. Telegram → пользователю
   "Статья готова: <ссылка на черновик в WP редакторе>"
   Кнопки:
     [✅ Опубликовать]
     [✏️ Редактировал вручную]

8a. Нажал "Опубликовать"
    → меняем статус черновика на published в WordPress
    → Telegram: "Готово: <ссылка на статью>"
    → статус в БД: published

8b. Нажал "Редактировал вручную"
    → значит пользователь уже поправил текст в WP редакторе
    → публикуем (draft → publish)
    → Telegram: "Готово: <ссылка на статью>"
    → статус в БД: edited (для аудита)
```

## Статусы статьи

| Статус       | Описание                                      |
|-------------|-----------------------------------------------|
| `pending`    | Ключ взят, генерация ещё не началась          |
| `generating` | Claude работает над статьёй                   |
| `draft`      | Черновик создан в WordPress                   |
| `published`  | Опубликовано через бота без правок            |
| `edited`     | Опубликовано после ручного редактирования     |
| `failed`     | Ошибка на одном из этапов                     |

## Стек

| Компонент      | Технология                        |
|---------------|-----------------------------------|
| Язык          | Go 1.22+                          |
| LLM           | Claude (Anthropic SDK)            |
| База данных   | PostgreSQL                        |
| Ключевые слова | Google Sheets                    |
| Публикация    | WordPress REST API                |
| Бот           | Telegram Bot API                  |

## Переменные окружения

```env
ANTHROPIC_API_KEY=        # Claude API ключ
DATABASE_URL=             # PostgreSQL connection string
GOOGLE_CREDENTIALS_FILE=  # Путь к JSON ключу сервис аккаунта
SPREADSHEET_ID=           # ID таблицы Google Sheets
WP_URL=                   # URL WordPress сайта
WP_USER=                  # Логин WordPress
WP_APP_PASSWORD=          # Application Password из WP Admin
TELEGRAM_BOT_TOKEN=       # Токен бота от @BotFather
TELEGRAM_ALLOWED_USER_IDS= # Кому разрешено пользоваться ботом
```
