package dev.allyourbase

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

class TokenStoreTest {
    @Test
    fun `in-memory token store save read and clear`() {
        val store: TokenStore = InMemoryTokenStore()

        assertNull(store.accessToken())
        assertNull(store.refreshToken())

        store.save("access-1", "refresh-1")
        assertEquals("access-1", store.accessToken())
        assertEquals("refresh-1", store.refreshToken())

        store.clear()
        assertNull(store.accessToken())
        assertNull(store.refreshToken())
    }

    @Test
    fun `in-memory token store allows null token updates`() {
        val store: TokenStore = InMemoryTokenStore("a", "r")

        store.save(null, "next-refresh")
        assertNull(store.accessToken())
        assertEquals("next-refresh", store.refreshToken())
    }

    @Test
    fun `in-memory token store is safe under concurrent access`() {
        val store: TokenStore = InMemoryTokenStore()
        val pool = Executors.newFixedThreadPool(4)
        val latch = CountDownLatch(200)

        repeat(100) { idx ->
            pool.submit {
                store.save("a-$idx", "r-$idx")
                latch.countDown()
            }
            pool.submit {
                store.accessToken()
                store.refreshToken()
                latch.countDown()
            }
        }

        val completed = latch.await(3, TimeUnit.SECONDS)
        pool.shutdownNow()

        assertEquals(true, completed)
    }
}
