package groot.kit.log

/**
 * Tagged logging that costs nothing when off. Messages are lambdas, so no string is built
 * unless logging is enabled. Apps set `Log.enabled = BuildConfig.DEBUG` at startup;
 * errors are always logged.
 */
public object Log {
    @Volatile
    public var enabled: Boolean = false

    public fun d(tag: String, message: () -> String) {
        if (enabled) android.util.Log.d(tag, message())
    }

    public fun i(tag: String, message: () -> String) {
        if (enabled) android.util.Log.i(tag, message())
    }

    public fun w(tag: String, throwable: Throwable? = null, message: () -> String) {
        if (enabled) android.util.Log.w(tag, message(), throwable)
    }

    public fun e(tag: String, throwable: Throwable? = null, message: () -> String) {
        android.util.Log.e(tag, message(), throwable)
    }
}
