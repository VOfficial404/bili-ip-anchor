package io.github.vofficial404.biliipanchor

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

/* Android 12 起用系统的动态取色，更早的系统用 B 站粉（#FB7299）生成的配色 */

private val LightColors = lightColorScheme(
    primary = Color(0xFFA0405E),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFFFD9E1),
    onPrimaryContainer = Color(0xFF3F0019),
    inversePrimary = Color(0xFFFFB1C5),
    secondary = Color(0xFF74565E),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFFFD9E1),
    onSecondaryContainer = Color(0xFF2B151B),
    tertiary = Color(0xFF7C5635),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFFFDCC1),
    onTertiaryContainer = Color(0xFF2E1500),
    background = Color(0xFFFFF8F8),
    onBackground = Color(0xFF22191B),
    surface = Color(0xFFFFF8F8),
    onSurface = Color(0xFF22191B),
    surfaceVariant = Color(0xFFF2DDE1),
    onSurfaceVariant = Color(0xFF514346),
    surfaceTint = Color(0xFFA0405E),
    inverseSurface = Color(0xFF382E30),
    inverseOnSurface = Color(0xFFFEEDEF),
    outline = Color(0xFF837376),
    outlineVariant = Color(0xFFD5C2C5),
    surfaceBright = Color(0xFFFFF8F8),
    surfaceDim = Color(0xFFE7D6D9),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerLow = Color(0xFFFFF0F2),
    surfaceContainer = Color(0xFFFAEAEC),
    surfaceContainerHigh = Color(0xFFF4E4E6),
    surfaceContainerHighest = Color(0xFFEEDEE1),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFFFFB1C5),
    onPrimary = Color(0xFF60102F),
    primaryContainer = Color(0xFF812845),
    onPrimaryContainer = Color(0xFFFFD9E1),
    inversePrimary = Color(0xFFA0405E),
    secondary = Color(0xFFE3BDC5),
    onSecondary = Color(0xFF422930),
    secondaryContainer = Color(0xFF5A3F46),
    onSecondaryContainer = Color(0xFFFFD9E1),
    tertiary = Color(0xFFEFBD94),
    onTertiary = Color(0xFF48290C),
    tertiaryContainer = Color(0xFF623F20),
    onTertiaryContainer = Color(0xFFFFDCC1),
    background = Color(0xFF191113),
    onBackground = Color(0xFFEFDFE1),
    surface = Color(0xFF191113),
    onSurface = Color(0xFFEFDFE1),
    surfaceVariant = Color(0xFF514346),
    onSurfaceVariant = Color(0xFFD5C2C5),
    surfaceTint = Color(0xFFFFB1C5),
    inverseSurface = Color(0xFFEFDFE1),
    inverseOnSurface = Color(0xFF372E30),
    outline = Color(0xFF9E8C90),
    outlineVariant = Color(0xFF514346),
    surfaceBright = Color(0xFF413739),
    surfaceDim = Color(0xFF191113),
    surfaceContainerLowest = Color(0xFF140C0E),
    surfaceContainerLow = Color(0xFF22191B),
    surfaceContainer = Color(0xFF261D1F),
    surfaceContainerHigh = Color(0xFF31282A),
    surfaceContainerHighest = Color(0xFF3C3234),
)

/** Material 3 没有的几种状态色：成功、警告、进行中 */
@Immutable
data class ExtraColors(val success: Color, val warning: Color, val info: Color)

private val LightExtra = ExtraColors(success = Color(0xFF1E7B3A), warning = Color(0xFFB45309), info = Color(0xFF0369A1))
private val DarkExtra = ExtraColors(success = Color(0xFF6DD58C), warning = Color(0xFFF5B158), info = Color(0xFF7CC4F2))

val LocalExtraColors = staticCompositionLocalOf { LightExtra }

@Composable
fun AppTheme(content: @Composable () -> Unit) {
    val dark = isSystemInDarkTheme()
    val context = LocalContext.current
    val scheme = when {
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        dark -> DarkColors
        else -> LightColors
    }
    CompositionLocalProvider(LocalExtraColors provides if (dark) DarkExtra else LightExtra) {
        MaterialTheme(colorScheme = scheme, content = content)
    }
}
