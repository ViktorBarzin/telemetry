package com.viktorbarzin.telemetry

import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.Parameterized
import org.junit.runners.Parameterized.Parameters

@RunWith(Parameterized::class)
class AppHostTest(private val given: String, private val host: String) {
    companion object {
        @JvmStatic
        @Parameters(name = "{0}")
        fun cases(): List<Array<Any>> = listOf(
            arrayOf("tv.viktorbarzin.me", "tv.viktorbarzin.me"),
            arrayOf("https://tv.viktorbarzin.me", "tv.viktorbarzin.me"),
            arrayOf("https://tv.viktorbarzin.me/", "tv.viktorbarzin.me"),
            arrayOf(" tv.viktorbarzin.me ", "tv.viktorbarzin.me"),
        )
    }

    @Test
    fun hostAndPipelineHosts() {
        val config = Config(appHost = given, writeKey = "wk", release = "1.0.0")
        assertEquals(host, config.host)
        assertEquals("$host/_t/e/v1", config.apiHost)
        assertEquals("$host/_t/e/v1", config.cdnHost)
    }
}

@RunWith(Parameterized::class)
class EnvironmentTest(private val environment: String, private val production: Boolean) {
    companion object {
        @JvmStatic
        @Parameters(name = "{0} -> {1}")
        fun cases(): List<Array<Any>> = listOf(
            arrayOf("production", true),
            arrayOf("dev", false),
            arrayOf("", false),
            arrayOf("Production", false),
            arrayOf("staging", false),
        )
    }

    @Test
    fun onlyProductionSends() {
        val config = Config("tv.viktorbarzin.me", "wk", "1", environment = environment)
        assertEquals(production, config.isProduction)
    }
}

class ConfigTest {
    @Test
    fun defaultEnvironmentSendsNothing() {
        assertEquals(false, Config("tv.viktorbarzin.me", "wk", "1").isProduction)
    }

    @Test
    fun rejectsAPathInTheHost() {
        assertThrows(IllegalArgumentException::class.java) {
            Config("tv.viktorbarzin.me/app", "wk", "1")
        }
    }

    @Test
    fun rejectsABlankWriteKey() {
        assertThrows(IllegalArgumentException::class.java) {
            Config("tv.viktorbarzin.me", " ", "1")
        }
    }
}
