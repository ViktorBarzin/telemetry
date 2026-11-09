plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    `maven-publish`
}

// JitPack sets VERSION to the tag being built (android-vX.Y.Z or vX.Y.Z).
version = System.getenv("VERSION") ?: "0.1.0-SNAPSHOT"

android {
    namespace = "com.viktorbarzin.telemetry"
    compileSdk = 34

    defaultConfig {
        minSdk = 23
        consumerProguardFiles("consumer-rules.pro")
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    publishing {
        singleVariant("release") {
            withSourcesJar()
        }
    }
}

kotlin {
    jvmToolchain(17)
    explicitApi()
}

tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile>().configureEach {
    compilerOptions.allWarningsAsErrors.set(true)
}

dependencies {
    implementation(libs.segment.android)
    implementation(libs.sentry.android.core)
    // Properties are kotlinx JsonObjects, so callers compile against it.
    api(libs.kotlinx.serialization.json)

    testImplementation(libs.junit)
}

publishing {
    publications {
        register<MavenPublication>("release") {
            groupId = "com.github.ViktorBarzin"
            artifactId = "telemetry"
            version = project.version.toString()
            afterEvaluate {
                from(components["release"])
            }
        }
    }
}
