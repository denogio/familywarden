plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Release signing comes from the environment (CI secrets or a local keystore).
// A device-owner app can only be updated with the same key, so it must never change.
val keystorePath: String? = System.getenv("MAST_KEYSTORE")

android {
    namespace = "me.hojgaard.ulysses.mast"
    compileSdk = 35

    defaultConfig {
        applicationId = "me.hojgaard.ulysses.mast"
        minSdk = 30          // DevicePolicyManager.setAutoTimeEnabled
        targetSdk = 35
        versionCode = (System.getenv("MAST_VERSION_CODE") ?: "1").toInt()
        versionName = System.getenv("MAST_VERSION_NAME") ?: "0.1.0-dev"
    }

    signingConfigs {
        if (keystorePath != null) {
            create("release") {
                storeFile = file(keystorePath)
                storePassword = System.getenv("MAST_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("MAST_KEY_ALIAS") ?: "mast"
                keyPassword = System.getenv("MAST_KEY_PASSWORD") ?: System.getenv("MAST_KEYSTORE_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            if (keystorePath != null) signingConfig = signingConfigs.getByName("release")
        }
    }

    buildFeatures {
        buildConfig = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
    testOptions {
        unitTests.isReturnDefaultValues = true
    }
}

dependencies {
    implementation("androidx.work:work-runtime-ktx:2.9.1")
    implementation("org.bouncycastle:bcprov-jdk18on:1.78.1")
    // QR pairing without a camera permission (Google Play services code scanner)
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
