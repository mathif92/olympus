using System;

// Adapter adapts the uniform stdin/stdout contract to the user's
// `static string Handler(string eventJson)` in Program.cs.
public static class Adapter
{
    public static int Main()
    {
        string eventJson = Console.In.ReadToEnd();
        string result = Program.Handler(eventJson);
        Console.Write(result);
        return 0;
    }
}
