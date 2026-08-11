import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;

public class Launch {
    public static void main(String[] args) throws IOException {
        byte[] bytes;
        try (InputStream in = System.in) {
            bytes = in.readAllBytes();
        }
        String event = new String(bytes, StandardCharsets.UTF_8);
        System.out.print(Handler.handler(event));
    }
}
