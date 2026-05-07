package com.xmdm.launcher.kiosk

import android.content.Context
import android.content.Intent
import android.graphics.PixelFormat
import android.os.Build
import android.provider.Settings
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.widget.FrameLayout
import com.xmdm.launcher.MainActivity

class KioskAdminOverlayController(
    private val context: Context,
) {
    private val windowManager = context.getSystemService(WindowManager::class.java)
    private var overlayView: View? = null
    private val gestureTracker = KioskExitGestureTracker()
    private val marginPx: Int
        get() = (12 * context.resources.displayMetrics.density).toInt()
    private val statusBarInsetPx: Int
        get() {
            val resourceId = context.resources.getIdentifier("status_bar_height", "dimen", "android")
            return if (resourceId > 0) context.resources.getDimensionPixelSize(resourceId) else 0
        }
    private val sizePx: Int
        get() = (48 * context.resources.displayMetrics.density).toInt()

    fun update(shouldShow: Boolean) {
        if (shouldShow) {
            show()
        } else {
            hide()
        }
    }

    fun hide() {
        val view = overlayView ?: return
        overlayView = null
        gestureTracker.reset()
        runCatching {
            windowManager?.removeView(view)
        }.onFailure {
            android.util.Log.w("XmdmLauncher", "failed to remove kiosk admin overlay", it)
        }
    }

    private fun show() {
        if (overlayView != null) {
            return
        }
        if (windowManager == null || !canDrawOverlays()) {
            android.util.Log.w("XmdmLauncher", "kiosk admin overlay unavailable; canDrawOverlays=${canDrawOverlays()}")
            return
        }
        val hotspot = FrameLayout(context).apply {
            setBackgroundColor(android.graphics.Color.TRANSPARENT)
            alpha = 0.01f
            isClickable = true
            isFocusable = true
            importantForAccessibility = View.IMPORTANT_FOR_ACCESSIBILITY_NO
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.LOLLIPOP) {
                elevation = 1f
            }
            setOnClickListener {
                android.util.Log.w("XmdmLauncher", "kiosk admin overlay tap instance=${context.packageName}")
                if (gestureTracker.registerTap(System.currentTimeMillis())) {
                    android.util.Log.w("XmdmLauncher", "kiosk admin overlay opening menu")
                    context.startActivity(
                        Intent(context, MainActivity::class.java)
                            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP)
                            .putExtra(MainActivity.EXTRA_OPEN_KIOSK_ADMIN_MENU, true),
                    )
                }
            }
        }
        val params = WindowManager.LayoutParams(
            sizePx,
            sizePx,
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY
            } else {
                @Suppress("DEPRECATION")
                WindowManager.LayoutParams.TYPE_PHONE
            },
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                WindowManager.LayoutParams.FLAG_LAYOUT_IN_SCREEN or
                WindowManager.LayoutParams.FLAG_LAYOUT_NO_LIMITS,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.TOP or Gravity.START
            x = marginPx
            y = statusBarInsetPx + marginPx
        }
        runCatching {
            windowManager.addView(hotspot, params)
            overlayView = hotspot
            android.util.Log.w("XmdmLauncher", "kiosk admin overlay shown")
        }.onFailure {
            android.util.Log.w("XmdmLauncher", "failed to show kiosk admin overlay", it)
        }
    }

    private fun canDrawOverlays(): Boolean {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            Settings.canDrawOverlays(context)
        } else {
            true
        }
    }
}
