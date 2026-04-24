package com.xmdm.launcher

import android.os.Bundle
import androidx.appcompat.app.AppCompatActivity
import com.xmdm.launcher.databinding.ActivityMainBinding

class MainActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)

        binding.launcherTitle.text = getString(R.string.launcher_title)
        binding.launcherStatus.text = getString(R.string.launcher_status)
    }
}
