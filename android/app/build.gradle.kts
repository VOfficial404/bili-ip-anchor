// 先编译界面（和桌面客户端同一份 MyGo 代码）：cd desktop && go tool mygo build -platform web，
// 再用 gomobile 把 Go 核心编成 app/libs/bia.aar：
//   gomobile bind -target=android/arm64,android/arm,android/amd64 -androidapi 24 -o android/app/libs/bia.aar ./mobile
// 最后构建：gradle -p android assembleRelease -PversionName=0.7.0 -PversionCode=700
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// 有签名密钥时用它签名，否则用调试密钥（每次构建的机器不同，覆盖安装会提示签名不一致）
val keystore: String? = System.getenv("ANDROID_KEYSTORE_FILE")

android {
    namespace = "io.github.vofficial404.biliipanchor"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.vofficial404.biliipanchor"
        minSdk = 24
        targetSdk = 34
        versionCode = (findProperty("versionCode") as String?)?.toInt() ?: 1
        versionName = (findProperty("versionName") as String?) ?: "dev"
    }

    signingConfigs {
        create("release") {
            if (keystore != null) {
                storeFile = file(keystore)
                storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("ANDROID_KEY_ALIAS")
                keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName(if (keystore != null) "release" else "debug")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }

    // 界面文件放在 assets/web，见下面的 copyWebUI
    sourceSets["main"].assets.srcDir(layout.buildDirectory.dir("webui").get().asFile)
}

// 把 desktop/build/web 复制进 APK 的 assets/web
val webUI = file("../../desktop/build/web")
val copyWebUI = tasks.register<Sync>("copyWebUI") {
    doFirst {
        check(File(webUI, "index.html").isFile) { "没有界面文件，先运行 cd desktop && go tool mygo build -platform web" }
    }
    from(webUI)
    into(layout.buildDirectory.dir("webui/web"))
}
tasks.named("preBuild") { dependsOn(copyWebUI) }

dependencies {
    implementation(files("libs/bia.aar"))
}
