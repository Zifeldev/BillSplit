# Billsplit — клиент (iOS, Android, Web)

Один Expo-проект (React Native + TypeScript, expo-router) для всех трёх платформ.
Приложение ходит в grpc-gateway бэкенда по HTTP/JSON:
`POST {API_URL}/billsplit.v1.<Service>/<Method>`.

## Запуск

```bash
cd apps/mobile
npm install
npx expo start
```

В терминале Expo:

- `w` — открыть веб-версию в браузере;
- `i` / `a` — iOS-симулятор / Android-эмулятор (если они установлены);
- на телефоне поставьте **Expo Go** и отсканируйте QR-код из терминала (iPhone — камерой, Android — из самого Expo Go). Телефон и Mac должны быть в одной Wi-Fi сети.

## Адрес бэкенда: `EXPO_PUBLIC_API_URL`

По умолчанию приложение обращается к:

- `http://localhost:8088` — веб и iOS-симулятор;
- `http://10.0.2.2:8088` — Android-эмулятор (так эмулятор видит localhost вашего Mac).

Для **физического телефона** `localhost` не подойдёт: укажите LAN-IP вашего Mac.
IP можно посмотреть командой `ipconfig getifaddr en0`.

```bash
EXPO_PUBLIC_API_URL=http://192.168.x.x:8088 npx expo start
```

Можно создать файл `apps/mobile/.env`:

```
EXPO_PUBLIC_API_URL=http://192.168.x.x:8088
```

После изменения переменной перезапустите `npx expo start` (при необходимости с флагом `-c`, чтобы сбросить кэш). Текущий адрес сервера виден в приложении: **Settings → About**.

Для веб-версии бэкенд должен разрешать CORS для адреса dev-сервера (обычно `http://localhost:8081`).

## Проверки

```bash
npx tsc --noEmit                   # типы
npx eslint src                     # линтер
npx expo export --platform web     # сборка веба в dist/
```

## Веб в Docker

`Dockerfile` собирает веб-версию в два этапа: Node (`npm ci` + `npx expo export --platform web`)
и nginx, который отдаёт `dist/` на порту 80. Конфиг nginx лежит в `nginx.conf`:

- SPA-фолбэк `try_files $uri /index.html`, поэтому прямые ссылки вида `/invite/<token>` и `/group/<id>` открываются;
- `/.well-known/apple-app-site-association` и `/.well-known/assetlinks.json` отдаются с `Content-Type: application/json`
  (файлы берутся из `public/.well-known/`).

Адрес API вшивается в бандл при сборке, поэтому его передают через build arg
(по умолчанию `http://localhost:8088`):

```bash
docker build -t billsplit-web --build-arg EXPO_PUBLIC_API_URL=https://api.billsplit.example.com apps/mobile
docker run --rm -p 8080:80 billsplit-web    # http://localhost:8080
```

Бэкенд должен разрешать CORS для домена, с которого открыт веб.

## Структура

```
src/
  api/          типизированный клиент: types.ts (зеркало proto), client.ts (fetch, refresh токена,
                нормализация int64 → number), services.ts (все RPC), queries.ts (react-query хуки и ключи),
                tokens.ts (SecureStore на телефоне, localStorage в вебе)
  auth/         AuthContext: сессия, логин/регистрация/логаут
  components/   ui.tsx (карточки, кнопки, поля, пикеры, состояния), charts.tsx (SVG-графики), ...
  lib/          деньги в минорных единицах и даты (format.ts), диалоги, шаринг, base64,
                push.ts / push.web.ts (expo-notifications), payments.ts (Venmo/PayPal/Zelle)
  theme/        светлая и тёмная палитры
  app/          экраны (expo-router)
    (auth)/login, (auth)/register
    (tabs)/index (Группы), friends, finance, settings
    group/new, group/[id], group/[id]/add-receipt, group/[id]/add-expense
    receipt/[id], receipt/[id]/edit
    finance/transactions, new-transaction, edit-transaction, import, budgets, goals, recurring
    notifications, invite/[token]
public/.well-known/   apple-app-site-association, assetlinks.json (заглушки)
Dockerfile, nginx.conf, .dockerignore   веб-сборка в nginx
app.config.js         переопределение домена Universal Links через APP_LINK_DOMAIN
```

## Ссылки-приглашения

Схема приложения — `billsplit`. Ссылки `billsplit://invite/<token>` открывают экран
`/invite/[token]`. В вебе работает и `https://<web-host>/invite/<token>`.
`GroupService/GetInvite` публичный, поэтому превью приглашения (группа, кто пригласил, сколько участников)
видно и без входа. Чтобы принять приглашение, нужно войти или зарегистрироваться: токен сохраняется,
и после входа экран приглашения открывается снова.

### Universal Links / App Links: домен

Чтобы `https://billsplit.example.com/invite/<token>` открывала установленное приложение, а не браузер, настроено:

- `app.json` → `ios.associatedDomains: ["applinks:billsplit.example.com"]`;
- `app.json` → `android.intentFilters`: `https`, host `billsplit.example.com`, `pathPrefix: /invite`, `autoVerify: true`.

**Как сменить домен.** Либо замените `billsplit.example.com` в двух местах `app.json`, либо задайте переменную
при сборке. `app.config.js` подставит её в обе настройки:

```bash
APP_LINK_DOMAIN=billsplit.myhost.com npx eas-cli@latest build -p ios
```

На этом домене должны отдаваться файлы из `public/.well-known/` (веб-контейнер из `Dockerfile` делает это сам).
В них остались заглушки, перед релизом их нужно заполнить:

- `apple-app-site-association`: вместо `TODO_TEAM_ID` подставьте Apple Team ID
  (developer.apple.com → Membership), получится `ABCDE12345.com.billsplit.app`;
- `assetlinks.json`: вместо `TODO:SHA256:...` подставьте SHA-256 отпечаток ключа подписи Android
  (`npx eas-cli@latest credentials -p android`, либо Play Console → App integrity → App signing).

Файлы отдаются по HTTPS, без редиректов, с `Content-Type: application/json`.
Universal/App Links работают только в собственной сборке (EAS или dev build), в Expo Go их нет:
там используйте `billsplit://` или веб.

## Push-уведомления и входящие

- После входа на **физическом устройстве** приложение просит разрешение на уведомления, получает Expo push token
  и вызывает `NotificationService/RegisterDevice` (`platform`: `ios` или `android`). При выходе вызывается `UnregisterDevice`,
  затем `AuthService/Logout` с refresh-токеном.
- Для токена нужен **EAS projectId** (`expo.extra.eas.projectId`). EAS-проекта пока нет, поэтому push отключены:
  в консоли будет предупреждение `[push] No EAS projectId`, остальное приложение работает. Чтобы включить push:
  `npx eas-cli@latest init` (projectId сам добавится в конфиг), потом настройте ключи APNs и FCM через `eas credentials`.
- Expo Go: на iOS push в Expo Go работают, если есть projectId. На Android, начиная с SDK 53, удалённые push
  в Expo Go не поддерживаются, нужен development build (`npx expo run:android` или `eas build --profile development`).
  В эмуляторах и симуляторах push не регистрируются.
- В вебе push нет, но входящие работают.
- **Входящие.** На вкладке «Группы» в шапке есть колокольчик со счётчиком непрочитанных. Пока вкладка открыта,
  `ListNotifications` опрашивается каждые 30 секунд. При открытии экрана уведомления помечаются прочитанными.
  По нажатию на уведомление (в списке или на push) открывается экран по `data`: `receipt_id` ведёт на чек,
  `group_id` на группу, всё остальное на вкладку «Друзья».

## Возврат долгов: Venmo, PayPal, Zelle

В **Settings → Get paid back** можно указать свои Venmo, PayPal.me и Zelle (email или телефон). Отправляются только
изменённые поля, пустая строка удаляет handle. Если вы должны человеку, у которого указаны handles, рядом с долгом
(в балансах группы и на вкладке «Друзья») появляются кнопки:

- **Venmo**: сначала `venmo://paycharge?txn=pay&recipients=<user>&amount=12.34&note=Billsplit: <группа>`,
  если приложение не установлено, то `https://venmo.com/<user>?txn=pay&amount=…&note=…`;
- **PayPal**: `https://paypal.me/<user>/<сумма><ВАЛЮТА>`;
- **Zelle**: handle копируется в буфер обмена, показывается короткая инструкция.

После перехода приложение спрашивает «Mark as paid?» и записывает оплату через `BalanceService/SettleUp`.
На вкладке «Друзья» долг общий по всем группам, а SettleUp работает внутри группы. Поэтому там погашаются
предложенные переводы «вы → друг» во всех общих группах; если таких переводов нет, приложение предложит отметить
оплату в самой группе. В группе кредитор может сам нажать «Mark as received» (`fromUserId` = должник).

## Суммы и валюты

Все `*Cents` поля передаются в **минорных единицах своей валюты**: у USD 2 знака, у JPY и KRW 0.
Число знаков берётся из `Intl.NumberFormat(…, {style:'currency', currency}).resolvedOptions().maximumFractionDigits`
(если не получилось, то 2). По нему и форматируются суммы, и разбирается ввод: `src/lib/format.ts`,
функции `formatMoney`, `parseMinor`, `minorToInput`.

## Примечания

- Суммы приходят как int64-строки и приводятся к числу в `api/client.ts`. Ключи внутри map-полей
  (`Notification.data`) не переименовываются.
- Access-токен живёт 15 минут. При ответе 401 клиент вызывает `AuthService/Refresh` и повторяет запрос.
  Refresh-токены одноразовые и ротируются: новый токен из каждого ответа сразу сохраняется, а параллельные
  401 ждут одного общего refresh-запроса. Если сервер отверг refresh-токен, клиент выходит из аккаунта.
  При сетевой ошибке сессия сохраняется.
- Публичные методы без токена: `Register`, `Login`, `Refresh`, `ListCurrencies`, `GetInvite`. Последние два отправляют токен,
  если он есть.
- Распознавание чека и импорт выписки асинхронные: экраны опрашивают сервер каждые 2 секунды.
