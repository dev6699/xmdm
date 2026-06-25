plugins {
    id("com.android.application") version "8.5.2"
    id("org.jetbrains.kotlin.android") version "1.9.24"
}

android {
    namespace = "com.xmdm.launcher"
    compileSdk = 34
    val testOnlyBuild = providers.gradleProperty("xmdm.testOnly")
        .orNull
        ?.toBooleanStrictOrNull()
        ?: true

    defaultConfig {
        applicationId = "com.xmdm.launcher"
        minSdk = 26
        targetSdk = 34
        versionCode = providers.gradleProperty("xmdm.versionCode")
            .orNull
            ?.toIntOrNull()
            ?: 1
        versionName = providers.gradleProperty("xmdm.versionName").orNull ?: "0.1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    buildFeatures {
        buildConfig = true
        viewBinding = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    signingConfigs {
        create("release") {
            val releaseKeystorePath = providers.gradleProperty("xmdm.release.keystore").orNull
            if (releaseKeystorePath != null) {
                storeFile = file(releaseKeystorePath)
                storePassword = providers.gradleProperty("xmdm.release.storePassword").orNull
                keyAlias = providers.gradleProperty("xmdm.release.keyAlias").orNull
                keyPassword = providers.gradleProperty("xmdm.release.keyPassword").orNull
            } else {
                // Keep CI and local release builds installable without secret material.
                initWith(getByName("debug"))
            }
        }
    }

    buildTypes {
        getByName("debug") {
            manifestPlaceholders["testOnly"] = if (testOnlyBuild) "true" else "false"
        }
        getByName("release") {
            signingConfig = signingConfigs.getByName("release")
            manifestPlaceholders["testOnly"] = "false"
        }
    }
}

dependencies {
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.datastore:datastore-preferences:1.1.1")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.4")
    implementation("com.google.android.material:material:1.12.0")
    implementation("com.google.code.gson:gson:2.11.0")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")

    testImplementation("junit:junit:4.13.2")
    testImplementation("androidx.datastore:datastore-preferences:1.1.1")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.8.1")
    androidTestImplementation("androidx.test.espresso:espresso-core:3.6.1")
    androidTestImplementation("androidx.test.ext:junit:1.2.1")
}
