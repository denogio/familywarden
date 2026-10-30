plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Release signing comes from the environment (CI secrets or a local keystore).
// A device-owner app can only be updated with the same key, so it must never change.
val keystorePath: String? = System.getenv("MAST_KEYSTORE")

android {
    namespace = "io.github.denogio.familywarden"
    compileSdk = 35

    defaultConfig {
        applicationId = "io.github.denogio.familywarden"
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
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
            if (keystorePath != null) signingConfig = signingConfigs.getByName("release")
        }
    }

    buildFeatures {
        buildConfig = true
        compose = true
    }

    packaging {
        // Both BouncyCastle and JSpecify contain an OSGi (not Android) manifest.
        resources.excludes += "META-INF/versions/9/OSGI-INF/MANIFEST.MF"
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
        unitTests.isIncludeAndroidResources = true
    }
}

dependencies {
    implementation(platform("androidx.compose:compose-bom:2025.06.00"))
    implementation("androidx.activity:activity-compose:1.10.1")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.compose.ui:ui-tooling-preview")
    debugImplementation("androidx.compose.ui:ui-tooling")
    implementation("androidx.work:work-runtime-ktx:2.9.1")
    implementation("org.bouncycastle:bcprov-jdk18on:1.78.1")
    // QR pairing without a camera permission (Google Play services code scanner)
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.robolectric:robolectric:4.14.1")
    testImplementation("org.json:json:20240303")
}
