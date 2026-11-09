# Applied to every app that depends on this library.
#
# sentry-android-core 8.63 refers to platform classes added after API 34
# (ApplicationStartInfo, ProfilingManager, ProfilingResult). An app that
# compiles against SDK 34 or lower cannot see them, so R8 fails the release
# build with "Missing class" until it is told they may be absent. Sentry only
# touches them behind a runtime version check. Found in tv's release build,
# 2026-11-09.
-dontwarn android.app.ApplicationStartInfo
-dontwarn android.os.ProfilingManager
-dontwarn android.os.ProfilingResult
