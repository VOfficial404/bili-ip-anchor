// 界面是原生的 Jetpack Compose，没有单独的界面构建步骤。
// 先用 gomobile 把 Go 核心编成 app/libs/bia.aar：
//   gomobile bind -target=android/arm64,android/arm,android/amd64 -androidapi 24 -o android/app/libs/bia.aar ./mobile
// 再构建：gradle -p android assembleRelease -PversionName=0.8.0 -PversionCode=800
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
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

    buildFeatures {
        compose = true
    }
}

dependencies {
    implementation(files("libs/bia.aar"))

    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-core")

    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
}
