#!/usr/bin/perl

use strict;
use Test::More;
use Test::Differences;
use lib 't';
use Test_Netspoc;

my ($title, $topo, $in, $out1, $head1, $out2, $head2, $out3, $head3);

############################################################
$title = "Protect interfaces of router";
############################################################

$in = <<END;
network:U = { ip = 10.1.1.0/24; }
router:R = {
 managed; 
 model = IOS;
 interface:U = { ip = 10.1.1.1; hardware = e0; }
 interface:N = { ip = 10.2.2.1; hardware = e1; }
}
network:N = { ip = 10.2.2.0/24; }

service:test = {
    user = network:U;
    permit src = user; dst = network:N; prt = tcp 80;
}
END

$out1 = <<END;
ip access-list extended e0_in
 deny ip any host 10.2.2.1
 permit tcp 10.1.1.0 0.0.0.255 10.2.2.0 0.0.0.255 eq 80
 deny ip any any
END

$out2 = <<END;
ip access-list extended e1_in
 deny ip any host 10.1.1.1
 permit tcp 10.2.2.0 0.0.0.255 10.1.1.0 0.0.0.255 established
 deny ip any any
END
$head1 = (split /\n/, $out1)[0];
$head2 = (split /\n/, $out2)[0];

eq_or_diff(get_block(compile($in), $head1, $head2), $out1.$out2, $title);

############################################################
$title = "Protect all interfaces";
############################################################

$in = <<END;
network:U = { ip = 10.1.1.0/24; }
router:R = {
 managed; 
 model = IOS;
 interface:U = { ip = 10.1.1.1; hardware = e0; }
 interface:N = { ip = 10.2.2.1; hardware = e1; }
}
network:N = { ip = 10.2.2.0/24; }

service:test = {
    user = network:U;
    permit src = user; dst = any:[network:N]; prt = tcp 80;
}
END

$out1 = <<END;
ip access-list extended e0_in
 deny ip any host 10.1.1.1
 deny ip any host 10.2.2.1
 permit tcp 10.1.1.0 0.0.0.255 any eq 80
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);

############################################################
$title = "Protect interfaces matching aggregate";
############################################################

$in = <<END;
network:U = { ip = 10.1.1.0/24; }
router:R = {
 managed; 
 model = IOS;
 interface:U = { ip = 10.1.1.1; hardware = e0; }
 interface:N = { ip = 10.2.2.1; hardware = e1; }
}
network:N = { ip = 10.2.2.0/24; }

service:test = {
    user = network:U;
    permit src = user; dst = any:[ip=10.2.0.0/16 & network:N]; prt = tcp 80;
}
END

$out1 = <<END;
ip access-list extended e0_in
 deny ip any host 10.2.2.1
 permit tcp 10.1.1.0 0.0.0.255 10.2.0.0 0.0.255.255 eq 80
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);

############################################################
$title = "Skip protection if permit any to interface";
############################################################

$in = <<END;
network:U = { ip = 10.1.1.0/24; }
router:R = {
 managed; 
 model = IOS;
 interface:U = { ip = 10.1.1.1; hardware = e0; }
 interface:N = { ip = 10.2.2.1; hardware = e1; }
}
network:N = { ip = 10.2.2.0/24; }

service:test = {
    user = network:U;
    permit src = user; dst = network:N; prt = tcp 80;
}

service:any = {
 user = any:[network:U];
 permit src = user; dst = interface:R.N; prt = ip;
}
END

$out1 = <<END;
ip access-list extended e0_in
 permit ip any host 10.2.2.1
 permit tcp 10.1.1.0 0.0.0.255 10.2.2.0 0.0.0.255 eq 80
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);

############################################################
$title = "Protect interfaces of crosslink cluster";
############################################################

$in = <<END;
network:U = { ip = 10.1.1.0/24; }
router:R1 = {
 managed; 
 model = IOS;
 interface:U = { ip = 10.1.1.1; hardware = e0; }
 interface:C = { ip = 10.9.9.1; hardware = e1; }
}
network:C = { ip = 10.9.9.0/29; crosslink; }
router:R2 = {
 managed; 
 model = IOS;
 interface:C = { ip = 10.9.9.2; hardware = e2; }
 interface:N = { ip = 10.2.2.1; hardware = e3; }
}
network:N = { ip = 10.2.2.0/24; }

service:test = {
    user = network:U;
    permit src = user; 
           dst = any:[network:N], any:[network:C]; 
           prt = tcp 80;
}
END

$out1 = <<END;
ip access-list extended e0_in
 deny ip any host 10.1.1.1
 deny ip any host 10.9.9.1
 deny ip any host 10.9.9.2
 deny ip any host 10.2.2.1
 permit tcp 10.1.1.0 0.0.0.255 any eq 80
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);

############################################################
$title = "Protect NAT interface";
############################################################

$in = <<END;
network:U = { ip = 10.1.1.0/24; }
router:R = {
 managed; 
 model = IOS;
 interface:U = { ip = 10.1.1.1; hardware = e0; bind_nat = N; }
 interface:N = { ip = 10.2.2.1; hardware = e1; }
}
network:N = { ip = 10.2.2.0/24; nat:N = { ip = 10.9.9.0/24; } }

service:test = {
    user = network:U;
    permit src = user; dst = network:N; prt = tcp 80;
}
END

$out1 = <<END;
ip access-list extended e0_in
 deny ip any host 10.9.9.1
 permit tcp 10.1.1.0 0.0.0.255 10.9.9.0 0.0.0.255 eq 80
 deny ip any any
END

$head1 = (split /\n/, $out1)[0];

eq_or_diff(get_block(compile($in), $head1), $out1, $title);

############################################################
done_testing;